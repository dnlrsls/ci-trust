package lintcatalog

// Artifact is compiled candidate provenance data, not runtime admission.
type Artifact struct {
	RepositoryID int64  `json:"repository_id"`
	RunID        int64  `json:"run_id"`
	Attempt      int    `json:"attempt"`
	ID           int64  `json:"artifact_id"`
	Size         int64  `json:"size"`
	Workflow     string `json:"workflow"`
	Revision     string `json:"revision"`
	Name         string `json:"name"`
	ZipSHA256    string `json:"zip_sha256"`
}

// CurrentArtifact identifies the existing HELD preparation artifact. Selecting
// this data cannot admit its old Node version or enable a network/load effect.
func CurrentArtifact() Artifact {
	return Artifact{
		RepositoryID: 1420154884, RunID: 39016088214, Attempt: 1, ID: 12094603849, Size: 65571165,
		Workflow:  ".github/workflows/lint-image-preparation.yml",
		Revision:  "88e8fd002e1e128fad0d35bdbc4572a1f7f7dad9",
		Name:      "lint-preparation-88e8fd002e1e128fad0d35bdbc4572a1f7f7dad9-39016088214-1",
		ZipSHA256: "sha256:67190310d485cc0c9d0633f0b64d6228da5ae91f0f48e8197bf91a91cce4545c",
	}
}
