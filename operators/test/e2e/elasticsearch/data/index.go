package data

import (
	"fmt"
	"strings"
)

func (d *Loader) setupIndex() error {
	m := fmt.Sprintf(mapping, d.shards, d.replicas)
	res, err := d.Client.Indices.Create(
		indexName,
		d.Client.Indices.Create.WithBody(
			strings.NewReader(m),
		),
		d.Client.Indices.Create.WithIncludeTypeName(false),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		return getErrorFromResponse(res)
	}
	return nil
}

func (d *Loader) ensureIndex() error {
	res, err := d.Client.Indices.Exists([]string{indexName})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() && res.StatusCode != 404 {
		return getErrorFromResponse(res)
	} else if res.StatusCode == 404 {
		return d.setupIndex()
	}
	return nil
}
