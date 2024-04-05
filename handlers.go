package main

import (
	"pkbldr/packages"
	"pkbldr/templates"
	"pkbldr/templates/pages"
	pages_packages "pkbldr/templates/pages/packages"
	"strconv"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/gofiber/fiber/v2"
)

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
	// Define template functions.
	metaTags := pages.MetaTags(
		"PikaOS, packages, package builder, build system", // define meta keywords
		"Welcome to the PikaOS Package Builder",           // define meta description
	)

	page := c.Query("page", "-1")
	isNotMainPage := true
	if page == "-1" {
		isNotMainPage = false
		page = "1"
	}
	pageInt, err := strconv.Atoi(page)
	if err != nil {
		pageInt = 1
	}
	packages := packages.GetPackagesSlice()
	if err != nil {
		return err
	}

	bodyContent := pages_packages.BodyContent(
		packages, pageInt, "/packages?page="+(strconv.Itoa(pageInt+1)), "/packages?page="+(strconv.Itoa(pageInt-1)))

	// Define template handler.
	templateHandler := templ.Handler(
		templates.Layout(
			"PikaOS Package Builder - Packages", // define title text
			metaTags, bodyContent, isNotMainPage,
		),
	)

	// Render template layout.
	return adaptor.HTTPHandler(templateHandler)(c)
}
