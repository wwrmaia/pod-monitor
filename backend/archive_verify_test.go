package main

// Teste de verificação manual da assinatura hand-rolled (SigV4/Shared Key)
// contra emuladores locais (MinIO / Azurite) — não faz parte da suíte
// permanente do projeto (que hoje não tem testes automatizados), é só pra
// confirmar que o algoritmo de assinatura está correto antes de confiar
// nele contra uma conta AWS/Azure real. Roda com:
//   MINIO_ENDPOINT=http://localhost:19000 AZURITE_ENDPOINT=http://localhost:19001 go test -run TestArchiveSignatures -v

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNFSArchiver(t *testing.T) {
	a := nfsArchiver{basePath: filepath.Join(t.TempDir(), "sub", "dir")}
	if err := a.Save(context.Background(), "cluster/2026-09-03.csv.gz", []byte("conteudo")); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(a.basePath, "cluster/2026-09-03.csv.gz"))
	if err != nil { t.Fatalf("arquivo não encontrado: %v", err) }
	if string(got) != "conteudo" { t.Fatalf("conteúdo divergente: %q", got) }
}

func TestArchiveSignatures(t *testing.T) {
	t.Run("S3_SigV4", func(t *testing.T) {
		endpoint := os.Getenv("MINIO_ENDPOINT")
		if endpoint == "" {
			t.Skip("MINIO_ENDPOINT não definido — pulei o teste contra o MinIO local")
		}
		a := s3Archiver{
			bucket: "pm-history-test", region: "us-east-1",
			endpoint: endpoint, accessKey: "testkey", secretKey: "testsecret123",
		}
		err := a.Save(context.Background(), "pod-monitor-history/verify/test.txt", []byte("assinatura sigv4 ok"))
		if err != nil && strings.Contains(err.Error(), "status 403") {
			t.Fatalf("SigV4 rejeitado (403 = SignatureDoesNotMatch) — bug na assinatura: %v", err)
		}
		if err != nil {
			t.Logf("gravação retornou erro não relacionado à assinatura (ok se só faltar criar o bucket): %v", err)
		} else {
			t.Log("gravação no MinIO bem-sucedida — assinatura SigV4 correta")
		}
	})

	t.Run("AzureBlob_SharedKey", func(t *testing.T) {
		endpoint := os.Getenv("AZURITE_ENDPOINT")
		if endpoint == "" {
			t.Skip("AZURITE_ENDPOINT não definido — pulei o teste contra o Azurite local")
		}
		// Conta/chave padrão bem conhecida do emulador Azurite (documentação
		// pública da Microsoft) — não é segredo real.
		a := azureBlobArchiver{
			account: "devstoreaccount1", container: "pm-history-test",
			accountKey: "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==",
			endpoint:   endpoint,
		}
		err := a.Save(context.Background(), "pod-monitor-history/verify/test.txt", []byte("assinatura shared key ok"))
		if err != nil && strings.Contains(err.Error(), "status 403") {
			t.Fatalf("Shared Key rejeitado (403 = AuthenticationFailed) — bug na assinatura: %v", err)
		}
		if err != nil {
			t.Logf("gravação retornou erro não relacionado à assinatura (ok se só faltar criar o container): %v", err)
		} else {
			t.Log("gravação no Azurite bem-sucedida — assinatura Shared Key correta")
		}
	})
}
