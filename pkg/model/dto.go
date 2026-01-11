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

type EnvelopeKey struct {
	TargetID    string `json:"targetId"`
	NamespaceID string `json:"namespaceId"`
	NskVersion  int    `json:"nskVersion"`
}

type Envelope struct {
	Key           EnvelopeKey `json:"key"`
	EncryptedBlob string      `json:"encryptedBlob"`
}
