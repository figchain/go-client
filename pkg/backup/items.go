package backup

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/figchain/go-client/pkg/model"
)

// backupItem is an item as the server writes it: {definition, latestFig}. A
// FigFamily-shaped item (figs/rules/defaultVersion) is also accepted.
type backupItem struct {
	Definition     model.FigDefinition `json:"definition"`
	Figs           []model.Fig         `json:"figs"`
	Rules          []model.Rule        `json:"rules"`
	DefaultVersion *string             `json:"defaultVersion"`
	LatestFig      *backupFig          `json:"latestFig"`
}

// backupFig is the server's Fig JSON, whose payload is nested under payload.data.binaryData.
type backupFig struct {
	FigID               string  `json:"figId"`
	Version             string  `json:"version"`
	Encrypted           bool    `json:"encrypted"`
	EncryptionAlgorithm *string `json:"encryptionAlgorithm"`
	KeyID               *string `json:"keyId"`
	WrappedDek          *string `json:"wrappedDek"`
	Payload             struct {
		Data struct {
			BinaryData *string `json:"binaryData"`
		} `json:"data"`
		Schema *PublicSchema `json:"schema"`
	} `json:"payload"`
}

// UnmarshalJSON maps the server's backup items to client FigFamilies and collects
// the schemas embedded in each latestFig.
func (p *BackupPayload) UnmarshalJSON(b []byte) error {
	var raw struct {
		TenantID    string       `json:"tenantId"`
		GeneratedAt string       `json:"generatedAt"`
		SyncToken   string       `json:"syncToken"`
		Items       []backupItem `json:"items"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	p.TenantID, p.GeneratedAt, p.SyncToken = raw.TenantID, raw.GeneratedAt, raw.SyncToken
	p.Items = make([]model.FigFamily, 0, len(raw.Items))
	for _, it := range raw.Items {
		ff := model.FigFamily{
			Definition:     it.Definition,
			Figs:           it.Figs,
			Rules:          it.Rules,
			DefaultVersion: it.DefaultVersion,
		}
		if lf := it.LatestFig; lf != nil {
			fig, err := lf.toFig()
			if err != nil {
				return err
			}
			ff.Figs = append(ff.Figs, fig)
			if ff.DefaultVersion == nil {
				v := fig.Version
				ff.DefaultVersion = &v
			}
			if s := lf.Payload.Schema; s != nil {
				if err := p.addSchema(it.Definition.SchemaURI, *s); err != nil {
					return err
				}
			}
		}
		p.Items = append(p.Items, ff)
	}
	return nil
}

func (f *backupFig) toFig() (model.Fig, error) {
	if f.Payload.Data.BinaryData == nil {
		return model.Fig{}, fmt.Errorf("latestFig %s has no payload.data.binaryData", f.FigID)
	}
	payload, err := base64.StdEncoding.DecodeString(*f.Payload.Data.BinaryData)
	if err != nil {
		return model.Fig{}, fmt.Errorf("latestFig %s: invalid binaryData: %w", f.FigID, err)
	}
	fig := model.Fig{
		FigID:               f.FigID,
		Version:             f.Version,
		Payload:             payload,
		IsEncrypted:         f.Encrypted,
		EncryptionAlgorithm: f.EncryptionAlgorithm,
		KeyID:               f.KeyID,
	}
	if f.WrappedDek != nil && *f.WrappedDek != "" {
		dek, err := base64.StdEncoding.DecodeString(*f.WrappedDek)
		if err != nil {
			return model.Fig{}, fmt.Errorf("latestFig %s: invalid wrappedDek: %w", f.FigID, err)
		}
		fig.WrappedDek = dek
	}
	return fig, nil
}

// addSchema records a schema's text keyed by URI. Content is base64 of the schema text;
// ContentText, when present, is the text itself.
func (p *BackupPayload) addSchema(fallbackURI string, s PublicSchema) error {
	uri := s.URI
	if uri == "" {
		uri = fallbackURI
	}
	if uri == "" {
		return nil
	}
	text := s.ContentText
	if text == "" && s.Content != "" {
		b, err := base64.StdEncoding.DecodeString(s.Content)
		if err != nil {
			return fmt.Errorf("schema %s: invalid content: %w", uri, err)
		}
		text = string(b)
	}
	if text == "" {
		return nil
	}
	if p.Schemas == nil {
		p.Schemas = make(map[string]string)
	}
	p.Schemas[uri] = text
	return nil
}
