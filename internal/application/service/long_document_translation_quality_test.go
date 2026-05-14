package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyLongDocumentTranslationBatchQualityGate_NormalizesMarkdownHeadingSpacing(t *testing.T) {
	content, err := applyLongDocumentTranslationBatchQualityGate("原文内容", "###3.1总体目标\n\n这是一个足够长的译文段落，用来验证标题和数字之间会被统一规范。")
	require.NoError(t, err)
	assert.Contains(t, content, "### 3.1 总体目标")
}

func TestValidateLongDocumentTranslationBatchOutput_RejectsHeaderFooterNoise(t *testing.T) {
	err := validateLongDocumentTranslationBatchOutput("source", "Confidential\n\n这是正常译文内容。\n\nConfidential")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "translation_batch_header_footer_noise")
}

func TestValidateLongDocumentTranslationBatchOutput_RejectsMalformedMarkdownTable(t *testing.T) {
	output := strings.Join([]string{
		"| 列一 | 列二 |",
		"| 数据一 | 数据二 |",
	}, "\n")
	err := validateLongDocumentTranslationBatchOutput("source", output)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "translation_batch_table_structure_invalid")
}
