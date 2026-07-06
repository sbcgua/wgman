#!/bin/bash
echo "Counting lines of code in Go files..."
find . -name "*.go" | xargs wc -l
echo -e "\nCounting lines of code in Markdown files..."
find . -name "*.md" | xargs wc -l
