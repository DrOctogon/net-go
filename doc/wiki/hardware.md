# VoiceWatch Hardware Recommendations

This document outlines the recommended hardware configurations for running VoiceWatch effectively, ensuring optimal performance for both detection capabilities and user interface responsiveness.

## Compute Hardware

### Recommended Systems

- **Primary Recommendation:** Raspberry Pi 5 (2GB RAM model is sufficient, 4GB recommended)
  - **Rationale:** The Pi 5 is highly recommended for a snappy web interface and good voice-detection inference performance.
  - **Processing Speed:** The Pi 5 processes audio chunks significantly faster than the Pi 4, allowing for more reliable detection at high overlap settings.

- **Alternative Systems:**
  - **Raspberry Pi 4B (4GB)** - Good performance, especially when overclocked to 1.8GHz
  - **Intel NUC** - Excellent performance, higher power consumption

- **Minimum Viable System:** Raspberry Pi 4B (2GB) or equivalent 64-bit ARM/x86 board
  - **Note:** While the Pi 4B can handle core voice detection well, expect slower web interface responsiveness compared to the Pi 5, and possible performance issues with Deep Detection and multiple simultaneous RTSP streams.
  - **Important:** Raspberry Pi 3 and Pi Zero are no longer supported. The codebase has outgrown these platforms.

### RAM Requirements

- **Baseline Usage:** ~400MB for core VoiceWatch processes
- **Recommended:**
  - 2GB RAM for standard installations (single audio source with BirdNET v2.4)
  - Each RTSP stream requires an additional FFmpeg process, which can consume substantial memory; if you have a high number of RTSP source streams consider a system with 4GB or more RAM

### Model-Specific Hardware Requirements

- **Silero VAD** (voice activity detection, default): Runs well on Raspberry Pi 4B (2GB) and above. Lightweight inference.
- **whisper.cpp transcription** (optional): Requires at least a Raspberry Pi 4 class system with 2GB of RAM. Larger models need more memory and CPU.

### CPU Considerations

- **Threads:** VoiceWatch can utilize multiple CPU cores. By default, it automatically optimizes for P-cores on hybrid architectures.
- **Performance Impact:** CPU power directly affects:
  - Maximum achievable overlap values (Deep Detection capability)
  - Spectrogram rendering speed
  - Multiple stream processing capacity
  - Audio export encoding speed

## Operating System

- **Recommended:** Raspberry Pi OS Lite (64-bit) based on Debian Bookworm (Debian 12)
  - **Rationale:**
    - 64-bit OS is **required** (not optional) because TensorFlow Lite requires a 64-bit system
    - "Lite" version avoids unnecessary desktop components, saving resources
    - Bookworm provides current package support for dependencies

- **Alternative OS Options:**
  - Ubuntu Server 22.04 LTS or newer (64-bit)
  - Debian 12 or newer (64-bit)
  - For desktop systems: Windows 10/11, macOS

## Audio Input Hardware

### Sound Cards

- **For voice detection:** USB audio interfaces with the following characteristics:
  - 48kHz sample rate support
  - Low self-noise
  - Proper line/mic input with appropriate gain control
  - Models to consider:
    - CM108 based USB sound card
    - Creative Sound Blaster Play! 3
    - U-Green USB sound card (multiple models)

### Microphones

**Important Note:** VoiceWatch processes all audio as mono. Always use mono microphones for optimal performance. Stereo microphones can introduce phase errors which may reduce detection accuracy.

#### DIY Microphone (Best Performance)

- **Recommended Capsule:** PUI Audio AOM-5024L-HD-R
  - **Characteristics:**
    - High sensitivity (-24dB)
    - Low self-noise
    - Omnidirectional pattern
    - Weather-resistant when properly housed

- **Construction Requirements:**
  - **Cable:** Shielded cable with braided copper or aluminum shield
    - Recommend: Mogami or Canare microphone cable
    - Cable length: Keep under 10m to minimize signal degradation
  - **Connector:** Quality metal 3.5mm TRS jack with proper shield connection
  - **Power:** Requires phantom power or bias voltage (can be supplied by many USB interfaces)

#### Pre-made Microphone Options

- **Lavalier (LAV) Microphones:**
  - Recommended models:
    - **Boya USB BY-LM40** (frequently recommended in the community), in reality not very sensitive meaning audio captured is quite low volume
    - Other omnidirectional lavalier microphones

## Storage

- **Primary Storage:** High Endurance MicroSD card
  - **Capacity:** 64GB or larger recommended
  - **Speed Rating:** **V30 rated or higher** to guarantee sufficient write performance
  - **Type Specification:** Look for cards explicitly marketed as:
    - "High Endurance"
    - "Surveillance Grade"
    - "Dashcam Rated"
  - **Recommended Brands:** SanDisk High Endurance, Samsung PRO Endurance, Western Digital Purple SC
  - **Rationale:** Standard MicroSD cards can wear out quickly due to constant read/write operations, leading to system failure. High Endurance cards are designed specifically for 24/7 recording applications.

- **Storage Utilization:**
  - Keep MicroSD card usage below 80% for optimal performance and longevity
  - Consider external USB storage for clip exports if extensive archiving is planned

- **Alternative Storage Options:**
  - **SSD Boot (Raspberry Pi 4/5):** Boot from USB SSD for improved reliability and performance
    - Requires Raspberry Pi OS 2021-01-11 or newer
    - Significantly improves system responsiveness and storage reliability
  - **NAS Storage:** Configure clip export to a network share for centralized storage
    - Requires stable network connection
    - Reduces wear on local storage

## Networking and Connectivity

- **Wired Ethernet Recommended:**
  - More reliable than Wi-Fi for 24/7 deployment
  - Required for maximum web interface performance, especially when viewing spectrograms
  - Essential for reliable RTSP stream ingestion

- **Wi-Fi Considerations:**
  - Use 5GHz networks when possible (less interference)
  - Position for strong signal (> -65dBm) if relying on Wi-Fi
  - Not recommended for critical deployments or multiple RTSP streams

- **Bandwidth Requirements:**
  - Basic web interface usage: ~1Mbps
  - Live audio streaming via web interface: ~0.5Mbps per stream
  - RTSP ingestion: Dependent on stream quality (typically 0.5-4Mbps per stream)

## Power Supply Considerations

- **Power Supply - IMPORTANT:**
  - **Only use the official Raspberry Pi power supply** matching your Pi model:
    - Pi 5: 5V/5A USB-C official power supply (27W PSU recommended)
    - Pi 4: 5V/3A USB-C official power supply
  - **Rationale for official PSU requirement:**
    - Official Pi PSUs deliver 5.1V (instead of standard 5V) to compensate for voltage drop
    - Better EMI/RFI shielding than third-party options, reducing audio interference
    - Properly rated for the current demands of the Pi under load
    - Using non-official PSUs may lead to under-voltage warnings and unstable operation

- **Power Consumption Estimates:**
  - Raspberry Pi 4B: ~4W (idle) to ~6.5W (active analysis)
  - Raspberry Pi 5: ~5W (idle) to ~8W (active analysis)
  - Add ~0.5-1W for USB audio interface and microphone

## Performance Optimization

### Hardware Optimizations

- **Raspberry Pi Specific:**
  - **Overclocking:** For Pi 4, consider modest overclocking to 1.8GHz for improved performance
    - Add to `/boot/firmware/config.txt`:
      ```
      over_voltage=2
      arm_freq=1800
      ```
  - **Memory Split:** Allocate minimum GPU memory (64MB) for headless installations
    - Add to `/boot/firmware/config.txt`:
      ```
      gpu_mem=64
      ```

- **Storage Optimizations:**
  - Consider disabling swap for SD card installations to reduce wear
  - For advanced users, move log files to RAM disk

### Software Configurations for Hardware Constraints

- **Limited CPU Resources:**
  - Reduce `birdnet.overlap` value (set to 0.0 for minimal CPU usage)
  - Limit the number of RTSP streams
  - Consider using `birdnet.threads` setting to reserve CPU resources for other tasks

- **Limited Memory Resources:**
  - Disable thumbnail generation in the web interface
  - Use WAV format for audio exports (requires less processing)
  - Implement aggressive retention policies for audio clips

## Additional Resources

- **Community Recommendations:**
  - Join the [VoiceWatch GitHub Discussions](https://github.com/tphakala/voicewatch/discussions) for user experiences with different hardware setups

- **Testing Your Setup:**
  - Use the built-in `benchmark` command to evaluate system performance:
    ```bash
    birdnet benchmark
    ```
  - Test microphone sensitivity with voice at measured distances
  - Compare detection rates across different hardware configurations

---

_This document represents recommended configurations based on community experience. Individual requirements may vary based on specific monitoring goals, environmental conditions, and budget constraints._
