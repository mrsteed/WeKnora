package router

import (
	"testing"

	handlerpkg "github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

func TestRegisterAuthRoutes_RegistersPreferencesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	RegisterAuthRoutes(v1, &handlerpkg.AuthHandler{})

	routes := r.Routes()
	want := map[string]bool{
		"PUT /api/v1/auth/me/preferences":   false,
		"PATCH /api/v1/auth/me/preferences": false,
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("missing auth route %s", key)
		}
	}
}
