package main

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/louis-she/simple-uploader/controllers"
	"github.com/spf13/viper"
)

func main() {
	viper.SetDefault("uploader.slice_cache_dir", "/tmp/golang_test_dev/cache")
	viper.SetDefault("uploader.upload_dir", "/tmp/golang_test_dev/data")
	viper.SetDefault("uploader.metafile_dir", "/tmp/golang_test_dev/meta")

	os.MkdirAll(viper.GetString("uploader.slice_cache_dir"), 0755)
	os.MkdirAll(viper.GetString("uploader.upload_dir"), 0755)
	os.MkdirAll(viper.GetString("uploader.metafile_dir"), 0755)

	r := gin.Default()
	controllers.Attach(r, "/")
	r.Run()
}
