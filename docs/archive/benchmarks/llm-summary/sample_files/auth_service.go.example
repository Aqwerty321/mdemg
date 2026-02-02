// Package auth provides authentication and authorization services.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User represents an authenticated user.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Roles        []string  `json:"roles"`
	CreatedAt    time.Time `json:"created_at"`
	LastLoginAt  time.Time `json:"last_login_at"`
}

// Session represents an active user session.
type Session struct {
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AuthService handles user authentication and session management.
type AuthService struct {
	userRepo     UserRepository
	sessionStore SessionStore
	tokenTTL     time.Duration
}

// NewAuthService creates a new authentication service.
func NewAuthService(userRepo UserRepository, sessionStore SessionStore) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		sessionStore: sessionStore,
		tokenTTL:     24 * time.Hour,
	}
}

// Login authenticates a user with email and password.
// Returns a session token on success.
func (s *AuthService) Login(ctx context.Context, email, password string) (*Session, error) {
	// Find user by email
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Generate session token
	token, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}

	// Create session
	session := &Session{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(s.tokenTTL),
	}

	// Store session
	if err := s.sessionStore.Save(ctx, session); err != nil {
		return nil, err
	}

	// Update last login
	user.LastLoginAt = time.Now()
	s.userRepo.Update(ctx, user)

	return session, nil
}

// ValidateToken validates a session token and returns the associated user.
func (s *AuthService) ValidateToken(ctx context.Context, token string) (*User, error) {
	// Look up session
	session, err := s.sessionStore.Get(ctx, token)
	if err != nil {
		return nil, errors.New("invalid session")
	}

	// Check expiration
	if time.Now().After(session.ExpiresAt) {
		s.sessionStore.Delete(ctx, token)
		return nil, errors.New("session expired")
	}

	// Get user
	user, err := s.userRepo.FindByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// Logout invalidates a session token.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	return s.sessionStore.Delete(ctx, token)
}

// HasRole checks if a user has a specific role.
func (s *AuthService) HasRole(user *User, role string) bool {
	for _, r := range user.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// generateSecureToken generates a cryptographically secure random token.
func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// UserRepository defines the interface for user data access.
type UserRepository interface {
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
}

// SessionStore defines the interface for session storage.
type SessionStore interface {
	Save(ctx context.Context, session *Session) error
	Get(ctx context.Context, token string) (*Session, error)
	Delete(ctx context.Context, token string) error
}
