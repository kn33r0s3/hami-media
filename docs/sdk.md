# SDK status

There is no supported client SDK. `sdk/web/hami.js` is an experimental
prototype, not a working integration:

- It calls `/api/publish/{room}` and `/api/subscribe/{room}`, which the
  server does not implement.
- The server's `/publisher` and `/viewer` handlers are private endpoints
  on an inherited loopback listener. They require a process Bearer token
  that must never be given to a browser, as well as a separate client
  HMAC token and identity/track fields.
- The prototype sends the process Bearer token as its `token` option and
  uses one PeerConnection for publish and subscribe, while the server
  expects separate publisher/viewer negotiations.

Do not expose the internal listener, pass its Bearer token to a client, or
present the prototype as a usable SDK. A supported Web SDK requires a
designed public signaling gateway and a matching authentication/session
contract first. See [api.md](api.md).

iOS/Android SDKs and sample apps are also not implemented; device testing
requires physical devices.
