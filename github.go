package main

type (
	Payload struct {
		After      string
		Before     string
		Commits    []Commit
		Compare    string
		Created    bool
		Deleted    bool
		Forced     bool
		HeadCommit Commit `json:"head_commit"`
		Pusher     Contributor
		Ref        string
		Repository Repository
	}

	Commit struct {
		Added     []string
		Author    Contributor
		Committer Contributor
		Distinct  bool
		ID        string
		Message   string
		Modified  []string
		Removed   []string
		Timestamp string
		URL       string
	}

	Contributor struct {
		Email    string
		Name     string
		Username string
	}

	Repository struct {
		CreatedAt    int `json:"created_at"`
		Description  string
		Fork         bool
		Forks        int
		HasDownloads bool `json:"has_downloads"`
		HasIssues    bool `json:"has_issues"`
		HasWiki      bool `json:"has_wiki"`
		Homepage     string
		ID           int
		Language     string
		MasterBranch string `json:"master_branch"`
		Name         string
		OpenIssues   int `json:"open_issues"`
		Owner        Contributor
		Private      bool
		PushedAt     int `json:"pushed_at"`
		Size         int
		Stargazers   int
		URL          string
		Watchers     int
	}
)
