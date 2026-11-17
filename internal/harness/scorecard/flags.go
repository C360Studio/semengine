package main

import (
	"flag"
	"fmt"
	"io"
	"regexp"

	"github.com/c360studio/semengine/graph/embedding/httpembedder"
)

// options are the program's flags. commit and tree come from Task, which reads them from git.
type options struct {
	url, model, queryPrefix string
	apiKey, apiKeyEnv       string
	provider                string
	commit, tree            string
}

var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

// parseOptions reads the flags and refuses, naming the cause, any it cannot run with: a missing
// URL or model, one the HTTP embedder refuses, an API key variable that is empty, a commit that
// is not a full hash, a tree that is not clean or dirty, or an argument that is not a flag.
func parseOptions(args []string, getenv func(string) string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("scorecard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.url, "url", "", "the provider's API root, as httpembedder's BaseURL (semembed's ends in /v1)")
	fs.StringVar(&o.model, "model", "", "the model name sent with every request")
	fs.StringVar(&o.queryPrefix, "query-prefix", "", "text put before each question, the model's query instruction")
	fs.StringVar(&o.apiKeyEnv, "api-key-env", "", "the environment variable holding the provider's API key")
	fs.StringVar(&o.provider, "provider", "", "a note on the provider, printed with the results")
	fs.StringVar(&o.commit, "commit", "", "the commit measured (set by task scorecard:tier1)")
	fs.StringVar(&o.tree, "tree", "", "clean or dirty (set by task scorecard:tier1)")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	switch {
	case fs.NArg() > 0:
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	case o.url == "":
		return options{}, fmt.Errorf("-url is required")
	case o.model == "":
		return options{}, fmt.Errorf("-model is required")
	case !fullCommit.MatchString(o.commit):
		return options{}, fmt.Errorf("-commit %q is not a full commit hash; run through task scorecard:tier1", o.commit)
	case o.tree != "clean" && o.tree != "dirty":
		return options{}, fmt.Errorf("-tree %q is not clean or dirty; run through task scorecard:tier1", o.tree)
	}
	if o.apiKeyEnv != "" {
		if o.apiKey = getenv(o.apiKeyEnv); o.apiKey == "" {
			return options{}, fmt.Errorf("-api-key-env %s: the variable is empty or unset", o.apiKeyEnv)
		}
	}
	// The embedder graph-embedding builds is this one; its refusal of the URL is the check.
	if _, err := httpembedder.New(o.embedderConfig()); err != nil {
		return options{}, err
	}
	return o, nil
}

func (o options) embedderConfig() httpembedder.Config {
	return httpembedder.Config{BaseURL: o.url, Model: o.model, APIKey: o.apiKey, QueryPrefix: o.queryPrefix}
}

func (o options) providerNote() string {
	if o.provider == "" {
		return "no note"
	}
	return o.provider
}
