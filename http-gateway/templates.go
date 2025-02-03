package main

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
)

var templates *template.Template

func initTemplates() {
	templates = template.Must(template.New("").
		ParseGlob("templates/*.html"))
}

func renderTemplate(w http.ResponseWriter, name string, data PageData) {
	content := templates.Lookup(name + "_content")
	if content == nil {
		log.Printf("Template %s_content not found", name)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	// Execute content template to string
	var contentBuf bytes.Buffer
	if err := content.Execute(&contentBuf, data); err != nil {
		log.Printf("Content template error: %v", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	// Create final data with rendered content
	finalData := PageData{
		Title:   data.Title,
		Content: template.HTML(contentBuf.String()),
	}

	// Execute base template
	err := templates.ExecuteTemplate(w, "base", finalData)
	if err != nil {
		log.Printf("Base template error: %v", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}
