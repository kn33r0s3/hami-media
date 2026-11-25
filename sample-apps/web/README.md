# Experimental web prototype

This page is retained as a UI/SDK prototype only. It is not a working
integration with the current server:

- The prototype calls `/api/publish/{room}` and
  `/api/subscribe/{room}`, which are not implemented.
- The server's `/publisher` and `/viewer` routes are internal,
  loopback-only proof endpoints. Their process Bearer token must never
  be sent to a browser.
- `node test/web-sdk-test.mjs` verifies mocked browser/network behavior;
  it does not verify compatibility with a live server.

The page's controls are disabled to prevent users from entering tokens
or mistaking the prototype for a supported sample. A working browser
sample requires a public signaling gateway and a defined client
authentication contract. See [`docs/sdk.md`](../../docs/sdk.md).
