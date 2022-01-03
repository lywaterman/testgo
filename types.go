package main

type immutable struct {
	Name string
	Namespace string
}

type DeployIndicator struct {
	immutable
}
