# Hosted Router Onboarding Video

This is the implementation-ready production package for the hosted-router
onboarding overview. All people, organizations, plans, model names, and status
values shown in the screens are synthetic.

- `mock/index.html` is the source for the nine 16:9 product mock screens. Open
  it with a `#s01` through `#s09` fragment to inspect a scene.
- `screens/` contains the Playwright captures used by the video.
- `audio/` contains the approved ElevenLabs narration clips.
- `hosted-router-onboarding.mp4` is the assembled video.
- `captions.srt` provides captions for the final narration.
- `production-manifest.md` records the approved timing, visual sequence, and
  non-secret production settings.

The mock is deliberately built as simple HTML/CSS rather than a generated
image so an implementation agent can inspect, reuse, and evolve the exact
interface concepts demonstrated in the video.
