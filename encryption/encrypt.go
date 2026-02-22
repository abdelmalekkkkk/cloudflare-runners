package encryption

import (
	"crypto/aes"
	"crypto/cipher"
)

type Sealed struct {
	Nonce      []byte
	Ciphertext []byte
	Tag        []byte
}

func Encrypt(data []byte, key []byte) (Sealed, error) {
	sealed := Sealed{}

	block, err := aes.NewCipher(key)
	if err != nil {
		return sealed, err
	}

	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return sealed, err
	}

	out := gcm.Seal(nil, nil, data, nil)

	size := len(out)

	sealed.Nonce = out[:12]
	sealed.Ciphertext = out[12 : size-16]
	sealed.Tag = out[size-16:]

	return sealed, nil
}
