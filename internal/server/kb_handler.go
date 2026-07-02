package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/JexKnox/mindbridge/internal/kb"
)

func (h *handlers) createKB(ctx context.Context, c *app.RequestContext) {
	var req kb.CreateRequest
	if err := c.Bind(&req); err != nil {
		writeErr(c, 400, "invalid body: "+err.Error())
		return
	}
	k, err := h.deps.KB.Create(ctx, req)
	if err != nil {
		writeErr(c, 500, err.Error())
		return
	}
	c.JSON(201, k)
}

func (h *handlers) listKB(ctx context.Context, c *app.RequestContext) {
	ks, err := h.deps.KB.List(ctx)
	if err != nil {
		writeErr(c, 500, err.Error())
		return
	}
	c.JSON(200, ks)
}

func writeErr(c *app.RequestContext, status int, msg string) {
	c.JSON(status, utils.H{"error": msg})
}
