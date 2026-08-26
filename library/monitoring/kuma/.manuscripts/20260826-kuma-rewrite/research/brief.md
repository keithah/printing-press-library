# Uptime Kuma research brief

The target deployment is Uptime Kuma v2. The supported integration surface observed in the live deployment is Socket.IO over HTTP: login, monitorList, heartbeatList, and monitor status events. `getHeartbeats` is emitted without a useful ACK and produces a streamed `heartbeatList` tuple, so the client must consume the push event. Monitor edits use full replacement semantics.

Live target used for acceptance: `uptime.hadm.net`.
