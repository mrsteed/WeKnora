package export

import "time"

const (
	FormatMarkdown = "markdown"
	FormatPDF      = "pdf"
	FormatDocx     = "docx"
	FormatXLSX     = "xlsx"

	markdownMaxContentBytes = 2 * 1024 * 1024
	documentMaxContentBytes = 1 * 1024 * 1024
)

type Policy struct {
	Format          string
	Engine          string
	Available       bool
	Reason          string
	MaxContentBytes int
	Timeout         time.Duration
}

type Capability struct {
	Available       bool   `json:"available"`
	Tool            string `json:"tool"`
	Engine          string `json:"engine"`
	Reason          string `json:"reason,omitempty"`
	MaxContentBytes int    `json:"max_content_bytes"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}

func SupportedFormats() []string {
	return []string{FormatMarkdown, FormatPDF, FormatDocx, FormatXLSX}
}

func PolicyForFormat(format string) Policy {
	switch format {
	case FormatMarkdown:
		return Policy{
			Format:          FormatMarkdown,
			Engine:          "builtin",
			Available:       true,
			MaxContentBytes: markdownMaxContentBytes,
			Timeout:         5 * time.Second,
		}
	case FormatPDF:
		available := IsChromiumAvailable()
		reason := ""
		if !available {
			reason = "chromium is not installed on the server"
		}
		return Policy{
			Format:          FormatPDF,
			Engine:          "chromium",
			Available:       available,
			Reason:          reason,
			MaxContentBytes: documentMaxContentBytes,
			Timeout:         45 * time.Second,
		}
	case FormatDocx:
		available := IsPandocAvailable()
		reason := ""
		if !available {
			reason = "pandoc is not installed on the server"
		}
		return Policy{
			Format:          FormatDocx,
			Engine:          "pandoc",
			Available:       available,
			Reason:          reason,
			MaxContentBytes: documentMaxContentBytes,
			Timeout:         30 * time.Second,
		}
	case FormatXLSX:
		return Policy{
			Format:          FormatXLSX,
			Engine:          "excelize",
			Available:       true,
			MaxContentBytes: documentMaxContentBytes,
			Timeout:         10 * time.Second,
		}
	default:
		return Policy{}
	}
}

func CapabilityForFormat(format string) Capability {
	policy := PolicyForFormat(format)
	return Capability{
		Available:       policy.Available,
		Tool:            policy.Engine,
		Engine:          policy.Engine,
		Reason:          policy.Reason,
		MaxContentBytes: policy.MaxContentBytes,
		TimeoutSeconds:  int(policy.Timeout / time.Second),
	}
}
