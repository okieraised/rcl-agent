package middlewares

import (
	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/utilities"
)

func ResponseHashMW() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		writer := &utilities.ResponseWriterInterceptor{
			ResponseWriter: ctx.Writer,
			Body:           make([]byte, 0),
		}
		ctx.Writer = writer
		ctx.Next()
	}
}
