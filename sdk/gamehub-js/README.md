# @igame/gamehub-js

Offline-friendly browser SDK for games hosted by igame. Authentication and all
server credentials stay in the portal; the SDK sends only the current session
cookie or a caller-supplied short-lived access token.

```ts
import { createGameHub } from '@igame/gamehub-js';

const hub = createGameHub({ gameId: 'snake' });
await hub.init();
await hub.start();
await hub.submitScore({ score: 3250, metadata: { level: 8 } });
await hub.finish({ score: 3250, duration: 185 });
```

The package includes `init`, `getUser`, `start`, `pause`, `resume`,
`submitScore`, `submitResult`, `unlockAchievement`, `getLeaderboard`,
`getEvent`, `finish`, and `telemetry`.
