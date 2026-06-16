package middleware

import (
	"net/http"
	"strings"

	"gohttpauto/internal/auth"
	"gohttpauto/internal/config"

	"github.com/gin-gonic/gin"
)

const CtxUserKey = "user"

func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		claims, err := auth.ParseToken(strings.TrimPrefix(h, "Bearer "), config.Global.JWTSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(CtxUserKey, claims)
		c.Next()
	}
}

func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			h := c.GetHeader("Authorization")
			if strings.HasPrefix(h, "Bearer ") {
				key = strings.TrimPrefix(h, "Bearer ")
			}
		}
		if key == "" || key != config.Global.APIKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}
		c.Set("triggered_by", "api")
		c.Next()
	}
}

func MasterOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		v, ok := c.Get(CtxUserKey)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		claims := v.(*auth.Claims)
		if claims.Role != "master" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "master role required"})
			return
		}
		c.Next()
	}
}
