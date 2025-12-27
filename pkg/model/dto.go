package model

type UserPublicKey struct {
	Email     string `json:"email"`
	PublicKey string `json:"publicKey"`
	Algorithm string `json:"algorithm"`
}

type NamespaceKey struct {
	WrappedKey string `json:"wrappedKey"`
	KeyID      string `json:"keyId"`
}
