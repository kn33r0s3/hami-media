# DeepAR effects

This folder is the home for the Hami Media DeepAR integration.

- The streamer offers lightweight browser color looks and the official
  DeepAR Aviators sample. The DeepAR SDK and effect load only when selected.
- Enter a DeepAR Web license key in the streamer page to activate the sample.
  Keys are not saved by Hami Media. DeepAR offers free test licenses; register
  the app's domain in the [developer portal](https://developer.deepar.ai/projects).
- Put only DeepAR `.deepar` effects that Hami Media is licensed to use in
  `effects/`.
- Put approved preview images in `previews/`.
- `catalog.json` is a shortlist of effect concepts for a first pilot. Its
  priorities are hypotheses, not verified trend or sales data. Other concepts
  are not usable until their licensed assets are supplied and added to the UI.

DeepAR receives the streamer's camera video, renders the processed result to a
canvas, and Hami publishes that canvas track with the original microphone track.
Viewers receive the already-processed video; they do not run the effect on the
remote stream.

The Web SDK is loaded on demand from the pinned `deepar@5.6.22` CDN package.
See the official [Web getting-started guide](https://docs.deepar.ai/deepar-sdk/platforms/web/getting-started),
[streaming guide](https://docs.deepar.ai/deepar-sdk/platforms/web/tutorials/streaming),
and [effects guide](https://docs.deepar.ai/deepar-sdk/filters). The SDK's
streaming guide recommends 640x360 camera input for performance; Hami requests
that while DeepAR is active and restores the camera constraints when it stops.

Room-wide filter notifications and approval voting are not implemented yet.
