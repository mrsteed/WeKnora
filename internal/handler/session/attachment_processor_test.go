package session

import (
	"context"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type attachmentProcessorFileServiceStub struct {
	interfaces.FileService
	path string
}

func (s *attachmentProcessorFileServiceStub) CheckConnectivity(context.Context) error { return nil }

func (s *attachmentProcessorFileServiceStub) SaveFile(context.Context, *multipart.FileHeader, uint64, string) (string, error) {
	return s.path, nil
}

func (s *attachmentProcessorFileServiceStub) SaveBytes(context.Context, []byte, uint64, string, bool) (string, error) {
	return s.path, nil
}

func (s *attachmentProcessorFileServiceStub) GetFile(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (s *attachmentProcessorFileServiceStub) GetFileURL(context.Context, string) (string, error) {
	return "", nil
}

func (s *attachmentProcessorFileServiceStub) DeleteFile(context.Context, string) error { return nil }

func (s *attachmentProcessorFileServiceStub) CopyFile(context.Context, string, uint64, string) (string, error) {
	return s.path, nil
}

func TestProcessAttachmentDetailed_RetainsFullTextWhilePreviewIsTruncated(t *testing.T) {
	lines := make([]string, 0, 520)
	for index := 1; index <= 520; index++ {
		lines = append(lines, "line-"+strconv.Itoa(index)+"-content-"+strings.Repeat("x", 4))
	}
	data := []byte(strings.Join(lines, "\n"))

	processor := NewAttachmentProcessor(
		&attachmentProcessorFileServiceStub{path: "local://10000/exports/attachment.txt"},
		nil,
		nil,
		nil,
	)

	result, err := processor.ProcessAttachmentDetailed(context.Background(), data, "attachment.txt", int64(len(data)), 10000, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Attachment)

	assert.True(t, result.Attachment.IsTruncated)
	assert.Equal(t, 520, result.Attachment.LineCount)
	assert.NotEmpty(t, result.FullText)
	assert.True(t, result.IsTextLike)
	assert.True(t, result.CanFastMaterialize)
	assert.Contains(t, result.FullText, lines[519])
	assert.Contains(t, result.Attachment.Content, lines[0])
	assert.NotContains(t, result.Attachment.Content, lines[519])
}
