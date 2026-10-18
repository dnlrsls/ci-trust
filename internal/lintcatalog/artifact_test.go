package lintcatalog

import "testing"

func TestCurrentArtifactIsDataNotAuthority(t *testing.T) {
	before := Current()
	a := CurrentArtifact()
	if a.RepositoryID != 1420154884 || a.RunID != 39016088214 || a.ID != 12094603849 || a.Attempt != 1 || a.Size != 65571165 || a.Revision != "88e8fd002e1e128fad0d35bdbc4572a1f7f7dad9" || a.Workflow != ".github/workflows/lint-image-preparation.yml" || a.ZipSHA256 != "sha256:67190310d485cc0c9d0633f0b64d6228da5ae91f0f48e8197bf91a91cce4545c" || a.Name != "lint-preparation-88e8fd002e1e128fad0d35bdbc4572a1f7f7dad9-39016088214-1" {
		t.Fatal("wrong compiled artifact")
	}
	a.ID++
	a.RunID++
	a.ZipSHA256 = "override"
	if a == CurrentArtifact() || Current() != before || BlockedReason() != "" {
		t.Fatal("caller copy granted authority")
	}
}
