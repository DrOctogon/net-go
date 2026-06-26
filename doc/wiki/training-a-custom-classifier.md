# Custom Classifier Training

VoiceWatch ships with a fixed **Silero VAD** (Voice Activity Detection) model that detects human voice as a single class. Unlike BirdNET-based deployments, VoiceWatch does not support plugging in custom classifiers, and custom classifier training is not applicable.

For the transcription layer (optional whisper.cpp), the model selection is handled through the Transcription settings in the web interface. No training is required or possible for the voice-activity detector itself.

If you are looking for the upstream BirdNET-Go documentation on training a custom BirdNET classifier for bird or sound detection, refer to the [BirdNET-Go project](https://github.com/tphakala/birdnet-go), from which VoiceWatch is forked.
