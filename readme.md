# simple uploader

This is a simple file uploader that supports resumable file uploads. It is provided as a controller of `Gin` web framework. Call `Attach` on `gin.Engine` to use it.

```go
import (
  ...
	"github.com/louis-she/simple-uploader/controllers"
  ...
)

r := gin.Default()

controllers.Attach(r, "/")  
```