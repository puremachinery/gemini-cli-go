#!/bin/sh
set -e

git config core.hooksPath .githooks

echo "Git hooks enabled via core.hooksPath=.githooks"
