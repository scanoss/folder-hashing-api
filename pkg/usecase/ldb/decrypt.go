package ldb

import (
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/salsa20"
)

func decrypt(key, nonce, ciphertext []byte) ([]byte, error) {

	out := make([]byte, len(ciphertext))
	salsa20.XORKeyStream(out, ciphertext, nonce, (*[32]byte)(key))
	return out, nil
}

var globalKey = []byte{
	0x1a, 0x0b, 0x5b, 0x5a, 0x5e, 0xce, 0x4b, 0x12,
	0x92, 0x48, 0x09, 0x65, 0x9f, 0xd8, 0x28, 0x7c,
	0xb4, 0xfc, 0x87, 0x38, 0x15, 0x41, 0x1b, 0xcb,
	0x59, 0x25, 0x4e, 0xe9, 0x28, 0x6c, 0x29, 0x75,
}

func DecodeTable(data []byte, table *TableDefinition, key []byte) ([]byte, error) {
	// Check if the table is encrypted
	if table.Definitions&LdbTableDefinitionEncrypted != 0 {

		nonce := make([]byte, CryptoStreamNonceBytes)

		//Nonce generation
		remains := CryptoStreamNonceBytes
		for i := 0; remains > 0; i++ {
			length := table.HashSize - 1
			if length > remains {
				length = remains
			}
			destPos := (table.HashSize - 1) * i
			copy(nonce[destPos:destPos+length], key[1:1+length])
			remains -= length
		}
		//we cant process compressed information from go
		if table.Definitions&LdbTableDefinitionCompressed != 0 {
			return nil, fmt.Errorf("not able to process compressed tables")
		}

		offset := table.HashSize * (table.KeysNumber - 1)
		inputData := data[offset:]
		msg, err := decrypt(globalKey, nonce, inputData)
		//return the decrypted information
		return msg, err
	}

	//If the table is not encrypte just return a copy of the message
	if (table.Definitions & LdbTableDefinitionEncrypted) == 0 {
		offset := table.HashSize * (table.KeysNumber - 1)
		return data[offset:], nil
	}

	return nil, nil
}

func DecodeString(in string, table *TableDefinition) ([]string, error) {
	touple := strings.SplitN(in, ",", table.KeysNumber+1)

	// Check if the table is encrypted
	key, err := hex.DecodeString(touple[0])
	if err != nil {
		return nil, err
	}

	inputData, err := hex.DecodeString(touple[table.KeysNumber])
	if err != nil {
		return nil, err
	}

	if table.Definitions&LdbTableDefinitionEncrypted != 0 {

		nonce := make([]byte, CryptoStreamNonceBytes)

		//Nonce generation
		remains := CryptoStreamNonceBytes
		for i := 0; remains > 0; i++ {
			length := table.HashSize - 1
			if length > remains {
				length = remains
			}
			destPos := (table.HashSize - 1) * i
			copy(nonce[destPos:destPos+length], key[1:1+length])
			remains -= length
		}
		//we cant process compressed information from go
		if table.Definitions&LdbTableDefinitionCompressed != 0 {
			return nil, fmt.Errorf("not able to process compressed tables")
		}

		msg, err := decrypt(globalKey, nonce, inputData)
		parts := strings.Split(string(msg), ",")
		result := append(touple[:table.KeysNumber], parts...)
		//return the decrypted information
		return result, err
	}

	//If the table is not encrypte just return a copy of the message
	if (table.Definitions & LdbTableDefinitionEncrypted) == 0 {
		parts := strings.Split(string(inputData), ",")
		result := append(touple[:table.KeysNumber], parts...)
		return result, err
	}

	return nil, nil
}
