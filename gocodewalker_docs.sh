#!/bin/bash

# Ensure the library is available (run from your codecat dir after go get)
echo "Attempting to fetch gocodewalker (if needed)..."
go get github.com/boyter/gocodewalker > /dev/null 2>&1
echo "--- Package Overview ---"
go doc github.com/boyter/gocodewalker
echo

echo "--- FileWalker Struct ---"
go doc github.com/boyter/gocodewalker.FileWalker
echo

echo "--- File Struct ---"
go doc github.com/boyter/gocodewalker.File
echo

echo "--- NewFileWalker Function ---"
go doc github.com/boyter/gocodewalker.NewFileWalker
echo

echo "--- Start Method ---"
go doc github.com/boyter/gocodewalker.FileWalker.Start
echo

echo "--- SetErrorHandler Method ---"
go doc github.com/boyter/gocodewalker.FileWalker.SetErrorHandler
echo

echo "--- Done ---"