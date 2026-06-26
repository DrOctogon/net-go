# VoiceWatch Detection Pipeline

This document explains how audio flows through VoiceWatch's voice-detection pipeline, from hardware capture to stored detections and notifications.

## Pipeline Overview

```mermaid
flowchart TB
    subgraph Capture["Audio Capture"]
        SC[Sound Card / ALSA]
        RTSP[RTSP / FFmpeg Stream]
    end

    subgraph Router["Audio Router"]
        AR[AudioRouter.Dispatch]
        R1[Route: VAD 16kHz]
        R2[Route: Full-Audio Rolling Capture]
    end

    subgraph Buffers["Buffers"]
        AB1[AnalysisBuffer\nSilero VAD\n30ms–96ms frame window\n16kHz]
        CB[CaptureBuffer\nRing buffer at source rate\nfor audio clip export]
    end

    subgraph Inference["Model Inference"]
        VAD[Silero VAD\nONNX\nHuman voice probability 0–1]
    end

    subgraph Processing["Detection Processing"]
        RQ[ResultsQueue\nchannel]
        PD[processDetections\nconfidence threshold check]
        PM[PendingDetections\nhold-and-count window]
        FL[Flush Cycle\n1s interval\ncheck deadlines]
    end

    subgraph Transcription["Optional Transcription"]
        TR[whisper.cpp\nSpeech-to-text]
        KF[Keyword Flagging\nkeyword list match]
    end

    subgraph Actions["Action Dispatch"]
        DB[(Database\nSQLite / MySQL)]
        SA[Save Audio Clip]
        SSE[SSE Broadcast\nWeb UI live feed]
        MQTT[MQTT Publish\nHome Assistant]
        LOG[Log File]
        CMD[Custom Command]
        ALERT[Alert Engine\nPush Notifications]
    end

    SC --> AR
    RTSP --> AR
    AR --> R1
    AR --> R2
    R1 -->|resample + EQ| AB1
    R2 --> CB

    AB1 -->|100ms poll| VAD
    VAD --> RQ

    RQ --> PD
    PD -->|above threshold| PM
    PM --> FL
    FL -->|approved| TR
    TR --> KF
    KF -->|flagged or plain| DB
    FL -->|approved| SA
    FL -->|approved| SSE
    FL -->|approved| MQTT
    FL -->|approved| LOG
    FL -->|approved| CMD
    DB -->|DetectionEvent| ALERT
```

## Stage 1: Audio Capture

Audio enters the system from two source types:

- **Sound cards** (via malgo/miniaudio): The `startCapture()` function opens the device and receives PCM frames in a hardware callback. Non-S16 formats (S24, S32, F32) are converted to 16-bit signed PCM. Each frame is wrapped in an `AudioFrame` with source ID, sample rate, bit depth, and timestamp.
- **RTSP streams** (via FFmpeg): An FFmpeg process decodes the network stream and produces the same `AudioFrame` type.

All frames are dispatched through the `AudioRouter`, which fans out each frame to registered routes via per-route buffered channels (capacity 64). Each route can apply gain adjustment and EQ filter chains.

## Stage 2: Buffering

The `AudioRouter` drives two parallel paths:

- **VAD Analysis Buffer**: Audio is resampled to 16 kHz (Silero VAD's required rate) and fed into an `AnalysisBuffer`. The buffer uses 30 ms or 96 ms frame windows (Silero VAD supports both).
- **CaptureBuffer**: A separate ring buffer stores raw audio at the original source sample rate. This buffer is not used for inference; its purpose is to provide audio clip data when a detection is confirmed and needs to be saved to disk.

The **Continuous Full-Audio Capture** feature supplements the clip-based `CaptureBuffer` with rolling chunks written to disk at configurable intervals, ensuring no audio is lost between trigger events.

## Stage 3: VAD Inference

An `analysisBufferMonitor` goroutine polls the VAD analysis buffer at 100 ms intervals. When a full frame is available:

1. Raw PCM bytes are converted to `[]float32`
2. `Orchestrator.PredictModel("SileroVAD", samples)` dispatches to the ONNX model
3. The model outputs a single probability score (0.0–1.0) representing the likelihood that the frame contains human speech
4. The result is sent to the shared `ResultsQueue` channel

### Sample Rate

| Model | Inference Rate | Source Capture Rate | Resampling |
|-------|---------------|-------------------|------------|
| Silero VAD | 16 kHz | Source rate (typically 48 kHz) | Downsampled by router |

## Stage 4: Detection Processing

```mermaid
flowchart TB
    RQ[ResultsQueue] --> DP[processDetections]

    subgraph Filtering["Per-Result Filters"]
        F1[Confidence threshold check\nglobal minimum]
    end

    DP --> F1

    F1 --> PM[Pending Detections\nhold-and-count window]

    PM --> WAIT[Wait for FlushDeadline\n= detection window duration]
    WAIT --> CHECK{Min detections\nmet?}
    CHECK -->|no| DISCARD[Discard]
    CHECK -->|yes| APPROVE[Approved Detection]
```

A single goroutine reads from `ResultsQueue` and applies filtering:

1. **Confidence threshold**: Frames where VAD probability falls below the configured minimum are discarded immediately.

### Hold-and-Count Window

Approved frames are held in a `PendingDetection` until the flush deadline. The key is `sourceID` (one pending detection per source at a time).

The pending detection tracks:
- `HitCount`: number of frames above threshold within the window
- `MaxConfidence`: highest probability seen in the window
- `FlushDeadline`: when to evaluate the detection for approval

At flush time, `HitCount` is checked against the minimum-detections threshold (derived from the `overlap`/`trigger` setting). If the count is sufficient, the detection is approved; otherwise it is discarded as a spurious noise burst.

## Stage 5: Transcription and Keyword Flagging (Optional)

When **Transcription** is enabled in settings, an approved detection triggers whisper.cpp to transcribe the audio clip:

1. The audio captured in `CaptureBuffer` for the detection window is passed to the whisper.cpp backend
2. whisper.cpp produces a text transcript
3. The **Keyword Flagging** stage checks the transcript against the user-configured keyword list (case-sensitive or case-insensitive depending on settings)
4. If a keyword matches, the detection is stored with a `keyword_flagged = true` marker and the matching keyword is recorded
5. If transcription is enabled but no keyword is matched, the detection is stored as a plain transcribed detection without a flag

Transcription is always performed on a confirmed voice detection — keyword flagging is the only step that adds extra metadata; it does not gate whether the detection is saved.

## Stage 6: Action Dispatch

When a pending detection's flush deadline arrives and it passes the minimum-detections check, it becomes an approved detection. The processor builds a list of actions and enqueues each as a `Task` on the `JobQueue`.

```mermaid
flowchart LR
    AD[Approved Detection] --> GA[getActionsForItem]

    GA --> COMP[CompositeAction\nsequential, per-step timeout]
    GA --> SAA[SaveAudioAction\nseparate from composite]

    COMP --> DBA[DatabaseAction\nSave to SQLite/MySQL]
    COMP --> SSEA[SSE Broadcast\nLive web feed]
    COMP --> MQTTA[MQTT Publish\nHome Assistant discovery]

    DBA -->|DetectionEvent| EB[Event Bus]
    EB --> ALRT[Alert Engine\nPush notifications\nWebhooks]

    GA --> LOGA[Log to File]
    GA --> CMDA[Custom Command]
```

**CompositeAction** wraps Database, SSE, and MQTT as sequential steps. `SaveAudioAction` runs separately to avoid slow FFmpeg encoding blocking the real-time broadcast.

### Audio Export

Audio is read from the `CaptureBuffer` ring buffer, which stores raw PCM at the original source rate. The clip covers the configured pre-capture buffer plus the detection window.

### JobQueue

The `JobQueue` is a bounded queue with retry/backoff support. MQTT actions have configurable retry policies. `SaveAudioAction` for Extended Capture uses exponential backoff (1s to 30s, up to ~20 minutes for long captures).

## Complete Data Flow Summary

```mermaid
flowchart TB
    subgraph Sources["Audio Sources"]
        SC["Sound Card\n48 kHz typical"]
        RTSP["RTSP Stream\n48 kHz typical"]
    end

    subgraph Capture["Capture Layer"]
        MALGO["malgo capture\nS16 PCM conversion"]
        FFMPEG["FFmpeg decode"]
    end

    AR["AudioRouter.Dispatch()"]

    subgraph Routes["Routes"]
        RVAD["Route: VAD\n16 kHz + EQ"]
        RCB["Route: CaptureBuffer\nsource rate"]
    end

    CB["CaptureBuffer\nRing buffer at source rate\nfor audio clip export"]

    subgraph AnalysisBuffers["Analysis Buffers"]
        AVAD["AnalysisBuffer\n30–96ms frames\n16 kHz"]
    end

    subgraph Monitors["Buffer Monitor (100ms poll)"]
        MVAD["Monitor: Silero VAD"]
    end

    subgraph Models["Model Inference"]
        VAD["Silero VAD\nONNX\nhuman voice probability"]
    end

    RQ["ResultsQueue"]

    subgraph Filtering["Detection Processing"]
        PD["processDetections()"]
        FC["Confidence threshold filter"]
        PM["PendingDetections map\nkey: sourceID\nhold-and-count window"]
        FL["flushPendingDetections()\n1s tick, check deadlines\nMin detections threshold"]
    end

    subgraph TranscriptionLayer["Optional Transcription"]
        TR["whisper.cpp\nspeech-to-text"]
        KF["Keyword Flagging\nkeyword list match"]
    end

    subgraph Actions["Action Dispatch"]
        DBA["Database\nSQLite / MySQL"]
        SAA["Save Audio\nfrom CaptureBuffer"]
        SSEA["SSE Broadcast\nWeb UI live feed"]
        MQTTA["MQTT Publish\nHome Assistant"]
        LOGA["Log File"]
        CMDA["Custom Command"]
        ALRT["Alert Engine\nPush Notifications"]
    end

    SC --> MALGO
    RTSP --> FFMPEG
    MALGO --> AR
    FFMPEG --> AR

    AR --> RVAD
    AR --> RCB
    RCB --> CB

    RVAD --> AVAD
    AVAD --> MVAD
    MVAD --> VAD
    VAD --> RQ

    RQ --> PD
    PD --> FC
    FC --> PM
    PM --> FL

    FL --> TR
    TR --> KF
    KF --> DBA
    FL --> SAA
    FL --> SSEA
    FL --> MQTTA
    FL --> LOGA
    FL --> CMDA
    DBA -->|DetectionEvent| ALRT
```
