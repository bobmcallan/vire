package blob

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestS3Store_ObjectKey(t *testing.T) {
	s := &S3Store{prefix: "vire", bucket: "my-bucket"}

	key := s.objectKey("filing_pdf", "BHP/20250101-doc.pdf")
	assert.Equal(t, "vire/filing_pdf/BHP/20250101-doc.pdf", key)
}

func TestS3Store_ObjectKeyNoPrefix(t *testing.T) {
	s := &S3Store{prefix: "", bucket: "my-bucket"}

	key := s.objectKey("filing_pdf", "BHP/20250101-doc.pdf")
	assert.Equal(t, "filing_pdf/BHP/20250101-doc.pdf", key)
}
