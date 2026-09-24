package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// createKnowledgeInFolder runs the file-upload path with the given
// path-qualified `fileName` (what the browser sends for a folder upload) and
// returns the persisted knowledge row.
func createKnowledgeInFolder(t *testing.T, uploadName, customFileName string) *types.Knowledge {
	t.Helper()

	repo := &createKnowledgeFileRepoStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   &createKnowledgeFileServiceStub{},
		task:      &createKnowledgeTaskEnqueuerStub{},
	}

	_, err := svc.CreateKnowledgeFromFile(
		newCreateKnowledgeFileContext(),
		"kb-1",
		newMultipartFileHeader(t, uploadName, "hello"),
		nil,
		nil,
		customFileName,
		nil,
		"",
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, repo.createdKnowledge)
	return repo.createdKnowledge
}

// createManualKnowledgeInFolder runs the manual-markdown creation path with the
// given folder_path and returns the persisted knowledge row.
func createManualKnowledgeInFolder(t *testing.T, folderPath string) *types.Knowledge {
	t.Helper()

	repo := &createKnowledgeFileRepoStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   &createKnowledgeFileServiceStub{},
		task:      &createKnowledgeTaskEnqueuerStub{},
	}

	_, err := svc.CreateKnowledgeFromManual(
		newCreateKnowledgeFileContext(),
		"kb-1",
		&types.ManualKnowledgePayload{
			Title:      "manual doc",
			Content:    "hello world",
			Status:     "draft",
			FolderPath: folderPath,
		},
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, repo.createdKnowledge)
	return repo.createdKnowledge
}

// A manual entry with a folder_path lands in that folder, mirroring the
// folder-upload behavior of the file path.
func TestCreateKnowledgeFromManual_FolderPathPersisted(t *testing.T) {
	t.Parallel()

	knowledge := createManualKnowledgeInFolder(t, "docs/spec")

	require.Equal(t, "docs/spec", knowledge.FolderPath)
	require.Equal(t, types.KnowledgeTypeManual, knowledge.Type)
}

// An empty folder path keeps the manual entry at the KB root.
func TestCreateKnowledgeFromManual_EmptyFolderPathStaysAtRoot(t *testing.T) {
	t.Parallel()

	knowledge := createManualKnowledgeInFolder(t, "  ")

	require.Empty(t, knowledge.FolderPath, "blank folder paths must stay at the KB root")
}

// Traversal-style segments (..) are stripped during normalization instead of
// escaping the KB scope; the remainder is still stored.
func TestCreateKnowledgeFromManual_FolderPathTraversalStripped(t *testing.T) {
	t.Parallel()

	knowledge := createManualKnowledgeInFolder(t, "../../docs/spec")

	require.Equal(t, "docs/spec", knowledge.FolderPath)
}

// A folder upload sends the browser's webkitRelativePath as `fileName`. The
// directory part must land in folder_path while file_name keeps only the base
// name, so the document list shows "design.md" instead of the whole path.
func TestCreateKnowledgeFromFile_FolderUploadSplitsPathAndName(t *testing.T) {
	t.Parallel()

	knowledge := createKnowledgeInFolder(t, "design.md", "docs/spec/design.md")

	require.Equal(t, "docs/spec", knowledge.FolderPath)
	require.Equal(t, "design.md", knowledge.FileName)
	require.Equal(t, "design.md", knowledge.Title)
	require.Equal(t, "md", knowledge.FileType)
}

func TestCreateKnowledgeFromFile_PlainUploadStaysAtRoot(t *testing.T) {
	t.Parallel()

	knowledge := createKnowledgeInFolder(t, "doc.txt", "")

	require.Empty(t, knowledge.FolderPath, "a single-file upload belongs to the KB root")
	require.Equal(t, "doc.txt", knowledge.FileName)
}

// A custom file name without any separator keeps working as a pure rename.
func TestCreateKnowledgeFromFile_CustomNameWithoutFolder(t *testing.T) {
	t.Parallel()

	knowledge := createKnowledgeInFolder(t, "upload.bin", "renamed.md")

	require.Empty(t, knowledge.FolderPath)
	require.Equal(t, "renamed.md", knowledge.FileName)
}

// Path traversal in the relative path must not escape the knowledge base root.
func TestCreateKnowledgeFromFile_FolderUploadRejectsTraversal(t *testing.T) {
	t.Parallel()

	knowledge := createKnowledgeInFolder(t, "design.md", "../../etc/docs/design.md")

	require.Equal(t, "etc/docs", knowledge.FolderPath)
	require.Equal(t, "design.md", knowledge.FileName)
}
