package middlewares

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/constants"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func RequestLoggingMW(logger *zap.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		start := time.Now()
		path := ctx.Request.URL.Path
		query := ctx.Request.URL.RawQuery
		ctx.Next()

		if len(ctx.Errors) > 0 {
			for _, e := range ctx.Errors.Errors() {
				logger.Error(e)
			}
			return
		}

		latency := time.Since(start).Milliseconds()
		fields := []zapcore.Field{
			zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
			//zap.String(constants.ContextFieldUsername, ctx.GetString(constants.ContextFieldUsername)),
			zap.Int("status", ctx.Writer.Status()),
			zap.String("method", ctx.Request.Method),
			zap.String("path", path),
			zap.String("message", path),
			zap.String("full-path", ctx.FullPath()),
			zap.String("query", query),
			zap.String("ip", ctx.ClientIP()),
			zap.String("user-agent", ctx.Request.UserAgent()),
			zap.Int64("latency", latency),
		}

		logger.Info("", fields...)
	}
}
