package cryptkeeper

import (
	"errors"
	"io"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/packet"
)

var config = &packet.Config{
	CompressionConfig:      &packet.CompressionConfig{Level: 3},
	DefaultCompressionAlgo: packet.CompressionZIP,
	DefaultCipher:          packet.CipherAES256,
}

var fileHints = &openpgp.FileHints{
	IsBinary: true,
}

type CryptKeeper struct {
	key []byte
}

func NewCryptKeeper(key string) *CryptKeeper {
	cryptKeeper := &CryptKeeper{
		key: []byte(key),
	}

	return cryptKeeper
}

func (this *CryptKeeper) Encrypt(input io.Reader, output io.Writer) error {

	plaintext, err := openpgp.SymmetricallyEncrypt(output, this.key, fileHints, config)
	if err != nil {
		return err
	}
	defer plaintext.Close()

	_, err = io.Copy(plaintext, input)
	if err != nil {
		return err
	}

	return nil
}

func (this *CryptKeeper) Decrypt(input io.Reader, output io.Writer) error {
	alreadyPrompted := false
	md, err := openpgp.ReadMessage(input, nil, func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
		// from openpgp docs: https://godoc.org/golang.org/x/crypto/openpgp#PromptFunction:
		// If the decrypted private key or given passphrase isn't correct, the function will be called again, forever.
		if alreadyPrompted {
			return nil, errors.New("Could not decrypt data using supplied passphrase")
		} else {
			alreadyPrompted = true
		}
		return this.key, nil
	}, config)
	if err != nil {
		return err
	}

	_, err = io.Copy(output, md.UnverifiedBody)
	if err != nil {
		return err
	}

	return nil
}
