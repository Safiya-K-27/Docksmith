package main

import "time"

type ImageRef struct {
	Name string
	Tag  string
}

type ImageConfig struct {
	Env        map[string]string `json:"Env"`
	Cmd        []string          `json:"Cmd"`
	WorkingDir string            `json:"WorkingDir"`
}

type LayerDescriptor struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	CreatedBy string `json:"createdBy"`
}

type ImageManifest struct {
	Name    string            `json:"name"`
	Tag     string            `json:"tag"`
	Digest  string            `json:"digest"`
	Created string            `json:"created"`
	Config  ImageConfig       `json:"config"`
	Layers  []LayerDescriptor `json:"layers"`
}

type CacheIndex struct {
	Entries map[string]string `json:"entries"`
}

type Instruction struct {
	Op    string
	Args  string
	Raw   string
	Line  int
	Stage int
}

type BuildOptions struct {
	TagRef  ImageRef
	Context string
	NoCache bool
}

type RuntimeOptions struct {
	ImageRef    ImageRef
	OverrideEnv map[string]string
	OverrideCmd []string
}

type StepLog struct {
	Step       int
	Text       string
	CacheState string
	Duration   time.Duration
}
