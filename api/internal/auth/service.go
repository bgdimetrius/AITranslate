package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

type tokenPair struct {
	accessToken   string
	refreshToken  string
	accessTokenID string
}

const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 7 * 24 * time.Hour
	tokenIssuer     = "ai_translate_api"
	tokenAudience   = "ai_translate_extension"
)

func decodeAccessToken(
	log *zap.Logger,
	accessToken string,
	secret string,
	withTime bool,
) (*claims, error) {
	parserOptions := []jwt.ParserOption{
		jwt.WithIssuer(tokenIssuer),
		jwt.WithAudience(tokenAudience),
		jwt.WithLeeway(5 * time.Second),
	}
	if !withTime {
		// During refresh, the expired access token is only a signed carrier for
		// the installation ID. The separately stored refresh-token hash provides
		// freshness; signature and signing-method checks still happen below.
		parserOptions = append(parserOptions, jwt.WithoutClaimsValidation())
	}

	token, err := jwt.NewParser(parserOptions...).ParseWithClaims(
		accessToken,
		&claims{},
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
			}
			return []byte(secret), nil
		},
	)
	if err != nil {
		log.Warn("couldn't decode access token", zap.Error(err))
		return nil, err
	}

	accessClaims, ok := token.Claims.(*claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid access token")
	}

	return accessClaims, nil
}

func generatePair(
	log *zap.Logger,
	userID int,
	secret string,
) (tokenPair, error) {
	if secret == "" {
		return tokenPair{}, errors.New("JWT secret is empty")
	}

	jwtIDBytes := make([]byte, 16)
	if _, err := rand.Read(jwtIDBytes); err != nil {
		log.Error("couldn't generate access token id", zap.Error(err))
		return tokenPair{}, err
	}
	accessTokenID := hex.EncodeToString(jwtIDBytes)

	now := time.Now()
	accessClaims := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Audience:  jwt.ClaimStrings{tokenAudience},
			Subject:   strconv.Itoa(userID),
			ID:        accessTokenID,
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	accessToken, err := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		accessClaims,
	).SignedString([]byte(secret))
	if err != nil {
		log.Error("couldn't sign access token", zap.Error(err))
		return tokenPair{}, err
	}

	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		log.Error("couldn't generate refresh token", zap.Error(err))
		return tokenPair{}, err
	}

	return tokenPair{
		accessToken:   accessToken,
		refreshToken:  hex.EncodeToString(randomBytes),
		accessTokenID: accessTokenID,
	}, nil
}

func hashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
