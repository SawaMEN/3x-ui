package service

import (
	"errors"
	"fmt"
)

// Reuse the existing validated renderer for built-in templates and custom HTML.
func managedCoverSiteOptions(catalog, template, html string) (*TelemtWebSiteOptions, error) {
	if (catalog == "") != (template == "") {
		return nil, errors.New("select both a cover site catalog and template")
	}
	if catalog == "" {
		if html != "" {
			return nil, errors.New("custom HTML requires the custom site template")
		}
		return nil, nil // legacy automatic page
	}
	var site *TelemtWebSiteOptions
	if catalog == "custom" && template == "html" {
		site = &TelemtWebSiteOptions{Mode: "html", HTML: html}
	} else {
		if html != "" {
			return nil, errors.New("custom HTML is only allowed with the custom site template")
		}
		site = &TelemtWebSiteOptions{Mode: "catalog", Catalog: catalog, Template: template}
	}
	if err := validateTelemtWebSite(site); err != nil {
		return nil, fmt.Errorf("invalid cover site: %w", err)
	}
	return site, nil
}
