# Simple Uploader

[![Test](https://github.com/louis-she/simple-uploader/actions/workflows/go.yml/badge.svg)](https://github.com/louis-she/simple-uploader/actions/workflows/go.yml)
[![Release](https://github.com/louis-she/simple-uploader/actions/workflows/release.yml/badge.svg)](https://github.com/louis-she/simple-uploader/actions/workflows/release.yml)

This is a simple file uploader that supports resumable file uploading. It is provided as a controller of the [`Gin Web Framework`](https://github.com/gin-gonic/gin).

Just call `Attach` on `gin.Engine` to add the routes and we are ready to go.

```go
import (
  ...
  "github.com/louis-she/simple-uploader/controllers"
  ...
)

r := gin.Default()

controllers.Attach(r, "/")  
```

# Clients

Only the Browser JavaScript client and Python client are provided. For Golang, see [`test`](/controllers/file_test.go).

All the clients expose very similar APIs.

### Browser JavaScript

**Installation**

```bash
npm config set @louis-she:registry https://npm.pkg.github.com/
npm install @louis-she/simple-uploader
```

**Usage**

```TypeScript
import SimpleUploader from "@louis-she/simple-uploader";

// file = ...

const uploader = new SimpleUploader(file, {
  endpoint: "/files",
  chunkSize: 10 * 1024 ** 2,  // 10 MB
  onProgress: (progress) => {
    // ...
  },
});
await uploader.upload();
const res = await uploader.checksum()
console.log(res)
```

For a more specific example, please see the development example [`main.ts`](/clients/browser_javascript/src/main.ts)

### Python

**Installation**

```bash
pip3 install git+https://github.com/louis-she/simple-uploader#subdirectory=clients/python
```

**Usage**

```python
from simple_uploader import SimpleUploader

su = SimpleUploader("/some/large/file/path")
su.upload()
res = su.checksum()
print(res)
```

### Development Client

1. Start mockserver

```bash
# in clients directory
go run ./mockserver
```

2. (For JS) Start dev server

```bash
# in clients/browser_javascript
pnpm isntall
pnpm run dev
```

## TODO

- [ ] Concurrent slice uploading
