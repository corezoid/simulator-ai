package publicapi

import "errors"

// A valid-looking success for another file is not confirmation of this write.
// Treat binding/count mismatches as uncertain outcomes and recover the receipt.
func validateWriteBinding(input smartRequest, value any) error {
	invalid := errors.New("Smart Form write result binding mismatch")
	switch p := input.(type) {
	case *CreateFoldersInput:
		r := value.(Folders)
		if len(r.Folders) != len(p.Items) {
			return invalid
		}
		for i, f := range r.Folders {
			want := p.Items[i]
			if f.FolderID != want.FolderID || f.Title != want.Title || f.IsStatic != want.IsStatic {
				return invalid
			}
		}
	case *CreateFilesInput:
		r := value.(Files)
		if len(r.Files) != len(p.Items) {
			return invalid
		}
		for i, f := range r.Files {
			want := p.Items[i]
			if f.FolderID != want.FolderID || f.Title != want.Title || f.Type != want.Type || f.Source != want.Source {
				return invalid
			}
		}
	case *UpdateFilesInput:
		r := value.(Files)
		if len(r.Files) != len(p.Items) {
			return invalid
		}
		expected := make(map[int64]FileUpdate, len(p.Items))
		for _, item := range p.Items {
			expected[item.ID] = item
		}
		if len(expected) != len(p.Items) {
			return invalid
		}
		for _, f := range r.Files {
			want, ok := expected[f.ID]
			if !ok || f.Title != want.Title || f.Type != want.Type {
				return invalid
			}
			if want.FolderID != nil && f.FolderID != *want.FolderID {
				return invalid
			}
			if want.Source != nil && f.Source != *want.Source {
				return invalid
			}
			delete(expected, f.ID)
		}
	}
	return nil
}
