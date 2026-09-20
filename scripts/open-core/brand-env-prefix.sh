#!/bin/sh
if [ -z "${ZQK_BRAND_ENV_PREFIX_ROOT:-}" ]; then
	_brand_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
else
	_brand_root=$ZQK_BRAND_ENV_PREFIX_ROOT
fi
BRAND_CONFIG=
if [ -f "$_brand_root/config/zqk-local.yaml" ]; then
	BRAND_CONFIG="$_brand_root/config/zqk-local.yaml"
elif [ -f "$_brand_root/config/zqk.yaml" ]; then
	BRAND_CONFIG="$_brand_root/config/zqk.yaml"
fi
BRAND_EXE=zqk
if [ -n "$BRAND_CONFIG" ]; then
	_parsed=$(awk '/^[[:space:]]*executable_name:/{gsub(/["\047]/, "", $2); print $2; exit}' "$BRAND_CONFIG")
	if [ -n "$_parsed" ]; then
		BRAND_EXE=$_parsed
	fi
fi
BRAND_ENV_PREFIX=$(printf '%s' "$BRAND_EXE" | tr 'abcdefghijklmnopqrstuvwxyz-' 'ABCDEFGHIJKLMNOPQRSTUVWXYZ_')
if [ -z "$BRAND_ENV_PREFIX" ]; then
	BRAND_ENV_PREFIX=ZQK
fi
unset _brand_root _parsed
