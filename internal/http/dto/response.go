package dto

import "github.com/gin-gonic/gin"

type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

type ErrorResponse struct {
	Success bool      `json:"success"`
	Error   ErrorBody `json:"error"`
}

func WriteSuccess(c *gin.Context, status int, data interface{}, message string) {
	c.JSON(status, SuccessResponse{Success: true, Data: data, Message: message})
}

func WriteError(c *gin.Context, status int, code, message string, details interface{}) {
	c.JSON(status, ErrorResponse{Success: false, Error: ErrorBody{Code: code, Message: message, Details: details}})
}

