package helper

import (
    "bytes"
    "crypto/rand"
    "crypto/rsa"
    "crypto/sha256"
    "crypto/x509"
    "encoding/base64"
    "fmt"
    "log"
)

const ChunkSize = 100


func EncryptData(data string) ([]byte, []byte, error) {
    log.Printf("Starting encryption of data with length: %d", len(data))

    // Generate key pair
    privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to generate private key: %v", err)
    }
    log.Printf("Generated 2048-bit RSA key pair")

    // Convert data to bytes
    dataBytes := []byte(data)
    
    // Calculate number of chunks
    numChunks := (len(dataBytes) + ChunkSize - 1) / ChunkSize
    log.Printf("Will split data into %d chunks of maximum %d bytes each", numChunks, ChunkSize)
    
    // Create slice to hold all encrypted chunks
    var encryptedChunks [][]byte
    
    // Encrypt each chunk
    for i := 0; i < numChunks; i++ {
        start := i * ChunkSize
        end := start + ChunkSize
        if end > len(dataBytes) {
            end = len(dataBytes)
        }
        
        chunk := dataBytes[start:end]
        log.Printf("Processing chunk %d/%d - Size: %d bytes", i+1, numChunks, len(chunk))
        
        // Encrypt chunk
        encryptedChunk, err := rsa.EncryptPKCS1v15(
            rand.Reader,
            &privateKey.PublicKey,
            chunk,
        )
        if err != nil {
            log.Printf("Failed to encrypt chunk %d: %v", i+1, err)
            log.Printf("Chunk size: %d, Key size: %d bits", len(chunk), privateKey.Size()*8)
            return nil, nil, fmt.Errorf("failed to encrypt chunk %d (%d bytes): %v", i+1, len(chunk), err)
        }
        
        encryptedChunks = append(encryptedChunks, encryptedChunk)
        log.Printf("Chunk %d encrypted successfully", i+1)
    }
    
    // Combine chunks with base64 encoding to ensure safe delimiter handling
    var encodedChunks [][]byte
    for i, chunk := range encryptedChunks {
        encoded := make([]byte, base64.StdEncoding.EncodedLen(len(chunk)))
        base64.StdEncoding.Encode(encoded, chunk)
        encodedChunks = append(encodedChunks, encoded)
        log.Printf("Chunk %d base64 encoded, size: %d", i+1, len(encoded))
    }
    
    // Join with delimiter
    combinedData := bytes.Join(encodedChunks, []byte("||"))
    log.Printf("All chunks combined, final size: %d bytes", len(combinedData))
    
    // Convert private key to bytes
    privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
    log.Printf("Private key marshaled, size: %d bytes", len(privateKeyBytes))
    
    return combinedData, privateKeyBytes, nil
}

func DecryptData(encryptedData []byte, privateKeyBytes []byte) (string, error) {
    log.Printf("Starting decryption of data with length: %d", len(encryptedData))
    
    // Parse private key
    privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyBytes)
    if err != nil {
        return "", fmt.Errorf("failed to parse private key: %v", err)
    }
    log.Printf("Private key parsed successfully")
    
    // Split combined data into chunks
    encodedChunks := bytes.Split(encryptedData, []byte("||"))
    log.Printf("Split into %d encoded chunks", len(encodedChunks))
    
    // Decrypt each chunk
    var decryptedChunks [][]byte
    
    for i, encodedChunk := range encodedChunks {
        // Decode base64
        decodedLen := base64.StdEncoding.DecodedLen(len(encodedChunk))
        decoded := make([]byte, decodedLen)
        n, err := base64.StdEncoding.Decode(decoded, encodedChunk)
        if err != nil {
            return "", fmt.Errorf("failed to decode chunk %d: %v", i+1, err)
        }
        decoded = decoded[:n]
        
        log.Printf("Decrypting chunk %d, size after base64 decode: %d", i+1, len(decoded))
        decryptedChunk, err := rsa.DecryptPKCS1v15(
            rand.Reader,
            privateKey,
            decoded,
        )
        if err != nil {
            return "", fmt.Errorf("failed to decrypt chunk %d: %v", i+1, err)
        }
        decryptedChunks = append(decryptedChunks, decryptedChunk)
        log.Printf("Chunk %d decrypted successfully", i+1)
    }
    
    // Combine decrypted chunks
    decryptedData := bytes.Join(decryptedChunks, nil)
    log.Printf("All chunks decrypted and combined, final size: %d bytes", len(decryptedData))
    
    return string(decryptedData), nil
}

func ValidateHash(decryptedData string, originalHash []byte) bool {
    hash := sha256.Sum256([]byte(decryptedData))
    return bytes.Equal(hash[:], originalHash)
}
