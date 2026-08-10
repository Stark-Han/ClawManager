package services

import (
	"errors"
	"fmt"
	"time"

	"clawreef/internal/config"
	"clawreef/internal/models"
	"clawreef/internal/repository"
	"clawreef/internal/utils"
)

// AuthService defines the interface for authentication operations
type AuthService interface {
	Register(username, email, password string) (*models.User, error)
	Login(username, password string) (*TokenPair, error)
	RefreshToken(refreshToken string) (*TokenPair, error)
	GetCurrentUser(userID int) (*models.User, error)
	ChangePassword(userID int, currentPassword, newPassword string) error
}

// TokenPair holds access and refresh tokens
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// authService implements AuthService
type authService struct {
	userRepo  repository.UserRepository
	jwtConfig config.JWTConfig
}

// NewAuthService creates a new auth service
func NewAuthService(userRepo repository.UserRepository, jwtConfig config.JWTConfig) AuthService {
	return &authService{
		userRepo:  userRepo,
		jwtConfig: jwtConfig,
	}
}

// Register registers a new user
func (s *authService) Register(username, email, password string) (*models.User, error) {
	// Check if username already exists
	existingUser, err := s.userRepo.GetByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("failed to check username: %w", err)
	}
	if existingUser != nil {
		return nil, errors.New("username already exists")
	}

	// Check if email already exists
	existingUser, err = s.userRepo.GetByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("failed to check email: %w", err)
	}
	if existingUser != nil {
		return nil, errors.New("email already exists")
	}

	// Hash password
	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user := &models.User{
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         "user",
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// Login authenticates a user and returns tokens
func (s *authService) Login(username, password string) (*TokenPair, error) {
	// Get user by username
	user, err := s.userRepo.GetByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, errors.New("invalid username or password")
	}

	// Check if user is active
	if !user.IsActive {
		return nil, errors.New("account is disabled")
	}

	// Verify password
	if !utils.VerifyPassword(password, user.PasswordHash) {
		return nil, errors.New("invalid username or password")
	}

	// Update last login
	now := time.Now()
	user.LastLogin = &now
	if err := s.userRepo.Update(user); err != nil {
		return nil, fmt.Errorf("failed to update last login: %w", err)
	}

	// Generate tokens
	tokenPair, err := s.generateTokens(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	return tokenPair, nil
}

// RefreshToken refreshes the access token using a refresh token
func (s *authService) RefreshToken(refreshToken string) (*TokenPair, error) {
	// Validate refresh token
	claims, err := utils.ValidateToken(refreshToken, s.jwtConfig.Secret)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	// Check token type
	if claims.TokenType != "refresh" {
		return nil, errors.New("invalid token type")
	}

	// Get user
	user, err := s.userRepo.GetByID(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Check if user is active
	if !user.IsActive {
		return nil, errors.New("account is disabled")
	}

	// Generate new tokens
	tokenPair, err := s.generateTokens(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	return tokenPair, nil
}

// GetCurrentUser gets the current user by ID
func (s *authService) GetCurrentUser(userID int) (*models.User, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, errors.New("user not found")
	}
	return user, nil
}

// ChangePassword updates the current user's password
func (s *authService) ChangePassword(userID int, currentPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return errors.New("user not found")
	}

	if !utils.VerifyPassword(currentPassword, user.PasswordHash) {
		return errors.New("current password is incorrect")
	}

	passwordHash, err := utils.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user.PasswordHash = passwordHash
	user.UpdatedAt = time.Now()

	if err := s.userRepo.Update(user); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// generateTokens generates access and refresh tokens
func (s *authService) generateTokens(userID int) (*TokenPair, error) {
	// Generate access token
	accessToken, err := utils.GenerateToken(utils.TokenClaims{
		UserID:    userID,
		TokenType: "access",
	}, s.jwtConfig.Secret, time.Duration(s.jwtConfig.AccessExpiry)*time.Minute)
	if err != nil {
		return nil, err
	}

	// Generate refresh token
	refreshToken, err := utils.GenerateToken(utils.TokenClaims{
		UserID:    userID,
		TokenType: "refresh",
	}, s.jwtConfig.Secret, time.Duration(s.jwtConfig.RefreshExpiry)*time.Hour)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.jwtConfig.AccessExpiry * 60,
	}, nil
}
