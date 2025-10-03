package fcrypt

import (
	"fmt"

	"filippo.io/age"
)

func LoadPublicKey(key string) (*age.X25519Recipient, error) {
	ageRecipient, err := age.ParseX25519Recipient(key)
	if err != nil {
		return nil, fmt.Errorf("error parsing age public key='%s': %w", key, err)
	}

	return ageRecipient, nil
}

func LoadPrivateKey(key string) (*age.X25519Identity, error) {
	ageIdentity, err := age.ParseX25519Identity(key)
	if err != nil {
		return nil, fmt.Errorf("error parsing age private key: %w", err)
	}

	return ageIdentity, nil
}
