# Simple Uploader

[![Test](https://github.com/louis-she/simple-uploader/actions/workflows/go.yml/badge.svg)](https://github.com/louis-she/simple-uploader/actions/workflows/go.yml)

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

## Browser JavaScript

```TypeScript
import SimpleUploader from "./simple_uploader";

// file = ...

const uploader = new SimpleUploader(file, {
  endpoint: "/files",
  chunkSize: 10 * 1024 ** 2,  // 10 MB
  onProgress: (progress) => {
    // ...
  },
});
await uploader.upload();
```

For a more specific example, please see the development example [`main.ts`](/clients/browser_javascript/src/main.ts)

TODO:

- [ ] Python
- [ ] Go
