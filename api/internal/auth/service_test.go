package auth

import (
	"testing"

	"go.uber.org/zap"
)

// Distinct IDs allow a newly issued token to supersede an otherwise identical
// token created for the same installation in the same second.
func TestGeneratePairProducesUniqueAccessTokens(t *testing.T) {
	const secret = "01234567890123456789012345678901"

	firstPair, err := generatePair(zap.NewNop(), 42, secret)
	if err != nil {
		t.Fatalf("generate first token pair: %v", err)
	}
	secondPair, err := generatePair(zap.NewNop(), 42, secret)
	if err != nil {
		t.Fatalf("generate second token pair: %v", err)
	}

	if firstPair.accessToken == secondPair.accessToken {
		t.Fatal("immediately reissued access tokens are identical")
	}
	firstClaims, err := decodeAccessToken(zap.NewNop(), firstPair.accessToken, secret, true)
	if err != nil {
		t.Fatalf("decode first access token: %v", err)
	}
	secondClaims, err := decodeAccessToken(zap.NewNop(), secondPair.accessToken, secret, true)
	if err != nil {
		t.Fatalf("decode second access token: %v", err)
	}
	if firstClaims.ID == "" || secondClaims.ID == "" {
		t.Fatalf("JWT IDs = %q and %q; want both populated", firstClaims.ID, secondClaims.ID)
	}
	if firstClaims.ID == secondClaims.ID {
		t.Fatalf("JWT IDs are both %q; want distinct values", firstClaims.ID)
	}
	if firstClaims.ID != firstPair.accessTokenID || secondClaims.ID != secondPair.accessTokenID {
		t.Fatal("generated token pair does not expose the JWT ID embedded in its access token")
	}
}
