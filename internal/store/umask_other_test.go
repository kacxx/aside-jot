//go:build !unix

package store

func setUmask(m int) int { return m }
