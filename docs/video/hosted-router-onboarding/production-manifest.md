# Hosted Router Onboarding Production Manifest

Narration source: [approved narration](../hosted-router-onboarding-approval.md)

TTS provider: ElevenLabs. Voice: `Daniel - Steady Broadcaster`
(`onwK4e9ZLuTAKqWW03F9`). Model: `eleven_multilingual_v2`. Credentials were
loaded from the active shell environment and are not recorded here.

Rendered audio is the timing source of truth. Pause configuration: 0.30 seconds
outgoing hold plus 0.45 seconds image crossfade after every non-final scene.

| Scene | Narration start | Narration end | Visual | Pause after |
| --- | ---: | ---: | --- | ---: |
| S01 | 0.000 | 14.675 | `screens/s01.png` | 0.750 |
| S02 | 15.425 | 32.190 | `screens/s02.png` | 0.750 |
| S03 | 32.940 | 46.315 | `screens/s03.png` | 0.750 |
| S04 | 47.065 | 62.250 | `screens/s04.png` | 0.750 |
| S05 | 63.000 | 77.536 | `screens/s05.png` | 0.750 |
| S06 | 78.286 | 96.026 | `screens/s06.png` | 0.750 |
| S07 | 96.776 | 114.145 | `screens/s07.png` | 0.750 |
| S08 | 114.895 | 131.009 | `screens/s08.png` | 0.750 |
| S09 | 131.759 | 146.016 | `screens/s09.png` | 0.000 |

Final render: `hosted-router-onboarding.mp4`, 1920x1080 at 30 fps with H.264
video and AAC audio. Container duration is 146.196 seconds; the 0.180-second
difference from the measured 146.016-second audio timeline is 30 fps frame
quantization across still-image scenes. Captions are delivered as the adjacent
`captions.srt` sidecar. They are not burned into the MP4 because the local
FFmpeg build does not include the `subtitles` filter.
