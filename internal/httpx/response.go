package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorBody{Code: code, Message: message})
}

func Unauthorized(c *gin.Context, message string) {
	if message == "" {
		message = "unauthorized"
	}
	Error(c, http.StatusUnauthorized, "unauthorized", message)
}

func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, "bad_request", message)
}

func Internal(c *gin.Context, message string) {
	if message == "" {
		message = "internal server error"
	}
	Error(c, http.StatusInternalServerError, "internal", message)
}
