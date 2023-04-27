package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func Attach(r *gin.Engine, prefix string) {
	fileController := &FileController{}
	fileController.AddRoutes(r, prefix)
}

type BaseController struct{}

func (b *BaseController) Write(c *gin.Context, data interface{}, httpStatus int, code int, message string) {
	if code == 0 {
		code = httpStatus
	}
	if message == "" {
		message = http.StatusText(httpStatus)
	}

	c.JSON(httpStatus, gin.H{
		"code":    code,
		"message": message,
		"data":    data,
	})
}

func (b *BaseController) AddRoutes() {}
