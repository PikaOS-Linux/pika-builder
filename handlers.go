package main

import (
	"pkbldr/packages"
	"pkbldr/templates"
	"pkbldr/templates/pages"
	pages_packages "pkbldr/templates/pages/packages"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/gofiber/fiber/v2"
)

const pageSize = 250

// indexViewHandler handles a view for the index page.
func indexViewHandler(c *fiber.Ctx) error {

	// Define template functions.
	metaTags := pages.MetaTags(
		"PikaOS, packages, package builder, build system", // define meta keywords
		"Welcome to the PikaOS Package Builder",           // define meta description
	)

	packageCount := packages.GetPackagesCount()
	bodyContent := pages.BodyContent(
		strconv.Itoa(packageCount.Built+packageCount.Stale),
		strconv.Itoa(packageCount.Stale),
		strconv.Itoa(packageCount.Queued),
		strconv.Itoa(packageCount.Building),
		strconv.Itoa(packageCount.Missing),
		strconv.Itoa(packageCount.Error),
		packages.LastUpdateTime.Format("02-01-2006 15:04:05"),
	)

	// Define template handler.
	templateHandler := templ.Handler(
		templates.Layout(
			"PikaOS Package Builder - Home", // define title text
			metaTags, bodyContent, false,
		),
	)

	// Render template layout.
	return adaptor.HTTPHandler(templateHandler)(c)

}

func packagesPageHandler(c *fiber.Ctx) error {
	metaTags := pages.MetaTags(
		"PikaOS, packages, package builder, build system", // define meta keywords
		"Welcome to the PikaOS Package Builder",           // define meta description
	)

	page := c.Query("page", "1")
	statusFilter := c.Query("status", "all")
	nameFilter := c.Query("name", "")

	pageInt, err := strconv.Atoi(page)
	if err != nil {
		pageInt = 1
	}

	allPackages := packages.GetPackagesSlice()

	// Apply filters
	var filteredPackages []packages.PackageInfo
	for _, pkg := range allPackages {
		if (statusFilter == "all" || statusFilter == "" || string(pkg.Status) == statusFilter) &&
			(nameFilter == "" || strings.Contains(pkg.Name, nameFilter)) {
			filteredPackages = append(filteredPackages, pkg)
		}
	}

	// Pagination
	totalPackages := len(filteredPackages)
	start := (pageInt - 1) * pageSize
	end := pageInt * pageSize
	if start > totalPackages {
		start = totalPackages
	}
	if end > totalPackages {
		end = totalPackages
	}
	paginatedPackages := filteredPackages[start:end]

	nextPage := "/packages?page=" + strconv.Itoa(pageInt+1) + "&status=" + statusFilter + "&name=" + nameFilter
	prevPage := "/packages?page=" + strconv.Itoa(pageInt-1) + "&status=" + statusFilter + "&name=" + nameFilter

	bodyContent := pages_packages.BodyContent(
		paginatedPackages, pageInt, nextPage, prevPage, statusFilter, nameFilter)

	templateHandler := templ.Handler(
		templates.Layout(
			"PikaOS Package Builder - Packages", // define title text
			metaTags, bodyContent, true,
		),
	)

	return adaptor.HTTPHandler(templateHandler)(c)
}
