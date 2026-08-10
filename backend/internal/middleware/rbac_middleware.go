package middleware

import (
	"net/http"

	"clawreef/internal/models"
	"clawreef/internal/repository"
	"github.com/gin-gonic/gin"
)

// AdminAuth middleware checks if user is admin
type AdminAuth struct {
	userRepo repository.UserRepository
}

// NewAdminAuth creates a new admin auth middleware
func NewAdminAuth(userRepo repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Unauthorized",
			})
			c.Abort()
			return
		}

		// Get user from database
		user, err := userRepo.GetByID(userID.(int))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to get user",
			})
			c.Abort()
			return
		}

		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "User not found",
			})
			c.Abort()
			return
		}

		// Check if user is admin
		if user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Admin access required",
			})
			c.Abort()
			return
		}

		// Set user role in context
		c.Set("userRole", user.Role)
		c.Next()
	}
}

// RoleAuth middleware checks if user has required role
func RoleAuth(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("userRole")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Role not found",
			})
			c.Abort()
			return
		}

		role := userRole.(string)
		for _, allowedRole := range allowedRoles {
			if role == allowedRole {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "Insufficient permissions",
		})
		c.Abort()
	}
}

// SetUserInfo middleware sets user info in context
func SetUserInfo(userRepo repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("userID")
		if !exists {
			c.Next()
			return
		}

		user, err := userRepo.GetByID(userID.(int))
		if err != nil || user == nil {
			c.Next()
			return
		}

		c.Set("userRole", user.Role)
		c.Set("user", user)
		c.Next()
	}
}

// GetCurrentUser gets current user from context
func GetCurrentUser(c *gin.Context) *models.User {
	user, exists := c.Get("user")
	if !exists {
		return nil
	}
	return user.(*models.User)
}
