package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go-demo/internal/config"
	"go-demo/internal/db"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserExists         = errors.New("user already exists")
)

type Claims struct {
	jwt.RegisteredClaims
	Role string `json:"role"`
}

type Service struct {
	dbx *db.DB
	cfg config.Config
	log *slog.Logger
}

func NewService(dbx *db.DB, cfg config.Config, log *slog.Logger) *Service {
	return &Service{dbx: dbx, cfg: cfg, log: log}
}

func (s *Service) Register(ctx context.Context, username, email, password, createdBy string) (*db.User, error) {
	if username == "" || email == "" || password == "" {
		return nil, fmt.Errorf("missing required fields")
	}

	var count int64
	if err := s.dbx.Gorm.WithContext(ctx).
		Model(&db.User{}).
		Where("username = ? OR email = ?", username, email).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("check existing: %w", err)
	}
	if count > 0 {
		return nil, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &db.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		CreatedBy:    createdBy,
		UpdatedBy:    createdBy,
		Role:         "USER",
	}
	if err := s.dbx.Gorm.WithContext(ctx).Create(u).Error; err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

// CreateUser creates a new user with the specified role (for admin use)
func (s *Service) CreateUser(ctx context.Context, username, email, password, role, createdBy string) (*db.User, error) {
	if username == "" || email == "" || password == "" || role == "" {
		return nil, fmt.Errorf("missing required fields")
	}

	// Validate role exists
	var roleCount int64
	if err := s.dbx.Gorm.WithContext(ctx).
		Model(&db.Role{}).
		Where("code = ?", role).
		Count(&roleCount).Error; err != nil {
		return nil, fmt.Errorf("check role: %w", err)
	}
	if roleCount == 0 {
		return nil, fmt.Errorf("invalid role: %s", role)
	}

	// Check if user already exists
	var count int64
	if err := s.dbx.Gorm.WithContext(ctx).
		Model(&db.User{}).
		Where("username = ? OR email = ?", username, email).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("check existing: %w", err)
	}
	if count > 0 {
		return nil, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &db.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		CreatedBy:    createdBy,
		UpdatedBy:    createdBy,
		Role:         role,
	}
	if err := s.dbx.Gorm.WithContext(ctx).Create(u).Error; err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (s *Service) Login(ctx context.Context, identifier, password string) (*db.User, string, time.Time, string, time.Time, error) {
	var u db.User
	if err := s.dbx.Gorm.WithContext(ctx).
		Where("username = ? OR email = ?", identifier, identifier).
		First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", time.Time{}, "", time.Time{}, ErrInvalidCredentials
		}
		return nil, "", time.Time{}, "", time.Time{}, fmt.Errorf("find user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, "", time.Time{}, "", time.Time{}, ErrInvalidCredentials
	}

	accessTok, accessExp, err := s.GenerateToken(u)
	if err != nil {
		return nil, "", time.Time{}, "", time.Time{}, err
	}
	refreshTok, refreshExp, err := s.GenerateRefreshToken(ctx, u.ID, u.Role)
	if err != nil {
		return nil, "", time.Time{}, "", time.Time{}, err
	}

	return &u, accessTok, accessExp, refreshTok, refreshExp, nil
}

func (s *Service) GenerateToken(u db.User) (string, time.Time, error) {
	if s.cfg.JWTSecret == "" {
		return "", time.Time{}, fmt.Errorf("JWT_SECRET is required")
	}
	ttl := s.cfg.JWTTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	exp := time.Now().Add(ttl)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Role: u.Role,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return ss, exp, nil
}

func (s *Service) ParseToken(tokenStr string) (string, error) {
	if s.cfg.JWTSecret == "" {
		return "", fmt.Errorf("JWT_SECRET is required")
	}
	parser := jwt.Parser{}
	claims := &Claims{}
	t, err := parser.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !t.Valid {
		return "", ErrInvalidCredentials
	}
	return claims.Subject, nil
}

func (s *Service) GetUserByID(ctx context.Context, id string) (*db.User, error) {
	var u db.User
	if err := s.dbx.Gorm.WithContext(ctx).First(&u, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

// GenerateRefreshToken creates and stores an opaque refresh token (hashed) for the user.
func (s *Service) GenerateRefreshToken(ctx context.Context, userID, role string) (string, time.Time, error) {
	ttl := s.cfg.RefreshTTL
	if ttl <= 0 {
		ttl = 720 * time.Hour // 30d default
	}
	exp := time.Now().Add(ttl)

	// Generate 32 random bytes and hex-encode (64 chars)
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", time.Time{}, fmt.Errorf("rand: %w", err)
	}
	plain := hex.EncodeToString(b[:])

	// Store sha256 hash
	sum := sha256.Sum256([]byte(plain))
	hash := hex.EncodeToString(sum[:])

	rt := &db.RefreshToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: exp,
	}
	if err := s.dbx.Gorm.WithContext(ctx).Create(rt).Error; err != nil {
		return "", time.Time{}, fmt.Errorf("store refresh token: %w", err)
	}
	return plain, exp, nil
}

// Refresh exchanges a valid refresh token for a new access token and a rotated refresh token.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*db.User, string, time.Time, string, time.Time, error) {
	if refreshToken == "" {
		return nil, "", time.Time{}, "", time.Time{}, ErrInvalidCredentials
	}

	// Hash input token
	sum := sha256.Sum256([]byte(refreshToken))
	hash := hex.EncodeToString(sum[:])

	var rt db.RefreshToken
	err := s.dbx.Gorm.WithContext(ctx).
		Where("token_hash = ?", hash).
		First(&rt).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", time.Time{}, "", time.Time{}, ErrInvalidCredentials
		}
		return nil, "", time.Time{}, "", time.Time{}, fmt.Errorf("find refresh token: %w", err)
	}
	if time.Now().After(rt.ExpiresAt) {
		// Expired: delete and reject
		_ = s.dbx.Gorm.WithContext(ctx).Delete(&rt).Error
		return nil, "", time.Time{}, "", time.Time{}, ErrInvalidCredentials
	}

	// Load user
	var u db.User
	if err := s.dbx.Gorm.WithContext(ctx).First(&u, "id = ?", rt.UserID).Error; err != nil {
		return nil, "", time.Time{}, "", time.Time{}, fmt.Errorf("load user: %w", err)
	}

	// Rotate: delete old, create new
	if err := s.dbx.Gorm.WithContext(ctx).Delete(&rt).Error; err != nil {
		return nil, "", time.Time{}, "", time.Time{}, fmt.Errorf("delete old refresh: %w", err)
	}
	newRefresh, newRefreshExp, err := s.GenerateRefreshToken(ctx, u.ID, u.Role)
	if err != nil {
		return nil, "", time.Time{}, "", time.Time{}, err
	}

	// Issue new access token
	access, accessExp, err := s.GenerateToken(u)
	if err != nil {
		return nil, "", time.Time{}, "", time.Time{}, err
	}

	return &u, access, accessExp, newRefresh, newRefreshExp, nil
}

// ListUsers returns a paginated list of users (for admin use)
func (s *Service) ListUsers(ctx context.Context, limit, offset int) ([]*db.User, int64, error) {
	var users []*db.User
	var total int64

	// Get total count
	if err := s.dbx.Gorm.WithContext(ctx).Model(&db.User{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// Get paginated users
	if err := s.dbx.Gorm.WithContext(ctx).
		Limit(limit).
		Offset(offset).
		Order("created_time DESC").
		Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}

	return users, total, nil
}

// UpdateUserStatus activates or deactivates a user by adding/removing an "active" field
// Since the current User model doesn't have an active field, we'll use a soft approach
// by updating the user's role to include "_INACTIVE" suffix for inactive users
func (s *Service) UpdateUserStatus(ctx context.Context, userID string, active bool, updatedBy string) (*db.User, error) {
	var user db.User
	if err := s.dbx.Gorm.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("find user: %w", err)
	}

	// Don't allow deactivating ADMIN users
	if user.Role == "ADMIN" {
		return nil, fmt.Errorf("cannot modify ADMIN user status")
	}

	// Update role based on active status
	var newRole string
	if active {
		// Remove _INACTIVE suffix if present
		if len(user.Role) > 9 && user.Role[len(user.Role)-9:] == "_INACTIVE" {
			newRole = user.Role[:len(user.Role)-9]
		} else {
			newRole = user.Role // Already active
		}
	} else {
		// Add _INACTIVE suffix if not present
		if len(user.Role) > 9 && user.Role[len(user.Role)-9:] == "_INACTIVE" {
			newRole = user.Role // Already inactive
		} else {
			newRole = user.Role + "_INACTIVE"
		}
	}

	// Update user
	if err := s.dbx.Gorm.WithContext(ctx).
		Model(&user).
		Updates(map[string]interface{}{
			"role":       newRole,
			"updated_by": updatedBy,
		}).Error; err != nil {
		return nil, fmt.Errorf("update user status: %w", err)
	}

	// Reload user to get updated data
	if err := s.dbx.Gorm.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		return nil, fmt.Errorf("reload user: %w", err)
	}

	return &user, nil
}

// DeleteUser soft deletes a user by updating their username/email to include deleted timestamp
func (s *Service) DeleteUser(ctx context.Context, userID, deletedBy string) error {
	var user db.User
	if err := s.dbx.Gorm.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("find user: %w", err)
	}

	// Don't allow deleting ADMIN users
	if user.Role == "ADMIN" {
		return fmt.Errorf("cannot delete ADMIN user")
	}

	// Soft delete by updating username and email to include timestamp
	timestamp := time.Now().Unix()
	deletedUsername := fmt.Sprintf("%s_deleted_%d", user.Username, timestamp)
	deletedEmail := fmt.Sprintf("%s_deleted_%d", user.Email, timestamp)

	if err := s.dbx.Gorm.WithContext(ctx).
		Model(&user).
		Updates(map[string]interface{}{
			"username":   deletedUsername,
			"email":      deletedEmail,
			"role":       "DELETED",
			"updated_by": deletedBy,
		}).Error; err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	// Delete all refresh tokens for this user
	if err := s.dbx.Gorm.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&db.RefreshToken{}).Error; err != nil {
		s.log.Error("failed to delete refresh tokens for deleted user", "user_id", userID, "err", err)
	}

	return nil
}

// IsUserActive checks if a user is active based on their role
func (s *Service) IsUserActive(user *db.User) bool {
	if user == nil {
		return false
	}
	// User is inactive if role ends with "_INACTIVE" or is "DELETED"
	if user.Role == "DELETED" {
		return false
	}
	if len(user.Role) > 9 && user.Role[len(user.Role)-9:] == "_INACTIVE" {
		return false
	}
	return true
}

// UpdateUserRole updates a user's role (for admin use)
func (s *Service) UpdateUserRole(ctx context.Context, userID, newRole, updatedBy string) (*db.User, error) {
	if userID == "" || newRole == "" || updatedBy == "" {
		return nil, fmt.Errorf("missing required fields")
	}

	// Validate that the new role exists
	var roleCount int64
	if err := s.dbx.Gorm.WithContext(ctx).
		Model(&db.Role{}).
		Where("code = ?", newRole).
		Count(&roleCount).Error; err != nil {
		return nil, fmt.Errorf("check role: %w", err)
	}
	if roleCount == 0 {
		return nil, fmt.Errorf("invalid role: %s", newRole)
	}

	// Find the user to update
	var user db.User
	if err := s.dbx.Gorm.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("find user: %w", err)
	}

	// Don't allow changing ADMIN users' roles
	if user.Role == "ADMIN" {
		return nil, fmt.Errorf("cannot modify ADMIN user role")
	}

	// Don't allow setting role to ADMIN
	if newRole == "ADMIN" {
		return nil, fmt.Errorf("cannot assign ADMIN role")
	}

	// Update the user's role
	if err := s.dbx.Gorm.WithContext(ctx).
		Model(&user).
		Updates(map[string]interface{}{
			"role":       newRole,
			"updated_by": updatedBy,
		}).Error; err != nil {
		return nil, fmt.Errorf("update user role: %w", err)
	}

	// Reload user to get updated data
	if err := s.dbx.Gorm.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		return nil, fmt.Errorf("reload user: %w", err)
	}

	return &user, nil
}
