package middlewares

import (
	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/cerrors"
)

func NoRouteMW() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		resp := api_response.New[any](ctx)
		resp.Populate(
			cerrors.ErrGenericUnknownAPIPath.Code,
			cerrors.ErrGenericUnknownAPIPath.Message,
			nil,
			nil,
			nil,
		)
		ctx.AbortWithStatusJSON(cerrors.ErrGenericUnknownAPIPath.HTTPStatus, resp)
		return
	}
}
