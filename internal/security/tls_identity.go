package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
	"yourdesk/internal/deviceid"
)

// 首次建立時混入硬體識別與安全隨機值；硬體識別不是秘密，不能單獨當私鑰。
func ServerTLSIdentity(dir string, hosts []string) (tls.Certificate, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return tls.Certificate{}, err
	}
	path := filepath.Join(dir, "identity.pem")
	if _, err := os.Stat(path); err == nil {
		if err = os.Chmod(path, 0600); err != nil {
			return tls.Certificate{}, err
		}
		certificate, err := tls.LoadX509KeyPair(path, path)
		if err != nil {
			return certificate, fmt.Errorf("TLS 身分檔損壞，不會自動重建：%w", err)
		}
		leaf, err := x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			return certificate, err
		}
		if time.Now().After(leaf.NotAfter) {
			return certificate, errors.New("TLS 憑證已過期，請執行受控換發")
		}
		for _, host := range hosts {
			if err = leaf.VerifyHostname(strings.TrimSpace(host)); err != nil {
				return certificate, err
			}
		}
		if err = os.WriteFile(filepath.Join(dir, "server-cert.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0644); err != nil {
			return certificate, err
		}
		return certificate, nil
	} else if !os.IsNotExist(err) {
		return tls.Certificate{}, err
	}
	identity, err := deviceid.Current()
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("首次建立 TLS 無法讀取硬體識別：%w", err)
	}
	entropy := make([]byte, 64)
	if _, err = rand.Read(entropy); err != nil {
		return tls.Certificate{}, err
	}
	material := append([]byte("YourDesk TLS identity v1\x00"+identity.UID+"\x00"), entropy...)
	seed := sha256.Sum256(material)
	private := ed25519.NewKeyFromSeed(seed[:])
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "YourDesk Server"}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.AddDate(5, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	for _, value := range hosts {
		host := strings.TrimSpace(value)
		if host == "" {
			return tls.Certificate{}, errors.New("TLS 主機名稱不可空白")
		}
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, private.Public(), private)
	if err != nil {
		return tls.Certificate{}, err
	}
	key, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return tls.Certificate{}, err
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	data := append(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), certificatePEM...)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return tls.Certificate{}, err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return tls.Certificate{}, err
	}
	if closeErr != nil {
		return tls.Certificate{}, closeErr
	}
	if err = os.WriteFile(filepath.Join(dir, "server-cert.crt"), certificatePEM, 0644); err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certificatePEM, data)
}
