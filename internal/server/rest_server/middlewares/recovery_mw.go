package middlewares

import (
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/cerrors"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
)

func RecoveryMW() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				resp := api_response.New[any](ctx)
				log.Default().Error(string(debug.Stack()))
				resp.Populate(
					cerrors.ErrGenericInternalServer.Code,
					cerrors.ErrGenericInternalServer.Message,
					&err,
					nil,
					nil)
				ctx.AbortWithStatusJSON(cerrors.ErrGenericInternalServer.HTTPStatus, resp)
				return
			}
		}()
		ctx.Next()
	}
}
