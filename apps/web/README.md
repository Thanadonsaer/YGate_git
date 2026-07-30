# Web

Next.js operator shell for the Solar SCADA platform.

## Configuration

- `NEXT_PUBLIC_GATEWAY_URL`: default `http://localhost:44440`

## Run

```powershell
npm install
npm run dev
```

The frontend listens on port `8080`. Browser API calls and the authenticated WebSocket connect through the Go API Gateway on port `44440`.

For the production-style native artifact:

```powershell
npm run build
npm start
```

The build's `postbuild` step packages Next.js static and public assets into `.next/standalone`; `npm start` serves that immutable artifact on port `8080`.