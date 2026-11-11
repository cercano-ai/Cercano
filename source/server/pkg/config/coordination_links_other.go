//go:build !windows

package config

import (
	"errors"
	"os"
	"reflect"
)

// configLinkCount reads Nlink from the Unix Stat_t behind info. Platforms
// whose FileInfo carries no Nlink fail closed.
func configLinkCount(_ string, info os.FileInfo) (uint64, error) {
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !stat.IsValid() || stat.Kind() != reflect.Struct {
		return 0, errors.New("cannot verify configuration link count")
	}
	links := stat.FieldByName("Nlink")
	if !links.IsValid() || !links.CanUint() {
		return 0, errors.New("cannot verify configuration link count")
	}
	return links.Uint(), nil
}
