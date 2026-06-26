# VoiceWatch Documentation

Welcome to the VoiceWatch documentation. This index will help you navigate through all available resources for installing, configuring, and using VoiceWatch.

## Getting Started

- [Frequently Asked Questions](faq.md) - Common questions, problems, and quick fixes
- [VoiceWatch Guide](guide.md) - Overview, configuration reference, and feature walkthroughs
- [Installation Guide](installation.md) - Comprehensive installation instructions for all methods
- [Recommended Hardware](hardware.md) - Hardware recommendations for optimal performance

## Installation Methods

- [Docker Installation (Linux)](installation.md#recommended-method-installsh-linux) - Using the automated `install.sh` script
- [Docker Compose Installation](docker_compose_guide.md) - Setting up VoiceWatch with Docker Compose
- [Manual Docker Installation](installation.md#manual-docker-installation-advanced-linux-only) - Advanced Docker setup
- [Manual Binary Installation](installation.md#manual-binary-installation-all-platforms) - Windows, macOS, and Linux binary installation

**Note**: Docker images are available from both [GitHub Container Registry](https://github.com/tphakala/voicewatch/pkgs/container/birdnet-go) and [Docker Hub](https://hub.docker.com/r/tphakala/birdnet-go).

## Advanced Features

- [External Media (USB, SD, File Shares)](external-media.md) - Mounting removable and network storage for import and backup
- [Detection Pipeline Architecture](detection-pipeline.md) - How audio flows through voice detection, transcription, keyword flagging, and action dispatch
- [Remote Internet Access](cloudflare_tunnel_guide.md) - Exposing VoiceWatch to the internet securely
- [Audio Processing](guide.md#audio-processing) - Advanced audio processing capabilities
- [Live Audio Streaming](guide.md#live-audio-streaming) - Streaming audio from the web interface
- [Continuous Full-Audio Recording](guide.md#audio-clip-retention) - Rolling audio capture and retention policies

## Integration & Security

- [Push Notifications](guide.md#push-notifications) - Discord, Telegram, Slack, and 20+ services
- [Discord Setup Guide](guide.md#discord-setup-guide) - Step-by-step Discord webhook configuration with rich embeds
- [MQTT Integration](guide.md#integration-options) - Connecting to IoT systems
- [Authentication](cloudflare_tunnel_guide.md#enabling-authentication) - Securing your VoiceWatch instance
- [Cloudflare Tunnel Setup](cloudflare_tunnel_guide.md) - Detailed guide for secure internet access

## Privacy & Telemetry

- [Error Tracking & Telemetry](telemetry.md) - Optional privacy-first error tracking system
- [Privacy & Data Collection](telemetry-privacy.md) - Detailed privacy information and data protection
- [Telemetry Setup Guide](telemetry-setup.md) - Step-by-step configuration instructions

## Troubleshooting & Support

- [Frequently Asked Questions](faq.md) - Common questions, problems, and quick fixes
- [RTSP Troubleshooting](rtsp-troubleshooting.md) - Comprehensive guide for RTSP camera issues and configuration
- [Docker Troubleshooting](guide.md#docker-installation-troubleshooting) - Resolving common Docker issues
- [Support Script](guide.md#support-script) - Generating diagnostic information
- [Reporting Issues](guide.md#reporting-issues) - How to effectively report bugs

## Reference

- [Command Line Interface](guide.md#command-line-interface) - Available commands and options
- [Detection Pipeline Flow](detection-pipeline.md) - How audio flows through VAD, transcription, and keyword flagging
- [Log Rotation](guide.md#log-rotation) - Managing log files
