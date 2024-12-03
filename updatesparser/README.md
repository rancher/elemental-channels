# Updatesinfo.xml file parsing tool

This little utility can be used to parse and filter updatesinfo.xml files
to generate changelogs based on packages match, dates and update type.

By default produces a human readable text output or a JSON following the
updatesinfo update structure. A custom template can also be provided to
fully customize the output.

## Custom templates

Any custom templates requires defining four different named golang templates: 

* `header`: This is only included once at the very begining of the output
* `body`: This is the representation of a single update entity in updatesinfo.xml files
* `join`: This is only included in between two updates (aka the separator character or lines between two updates)
* `footer`: This is only included once at the very end of the output

## Compile

```
make build
```

## Usage 

```
$ updatesparser --help
A simple CLI to parser updateinfo XML files

Usage:
  updatesparser [flags] updateinfo

Flags:
  -a, --afterDate string    Filter updates released after the given date. Date as a unix timestamp
  -b, --beforeDate string   Filter updates released before the given date. Date as a unix timestamp
  -h, --help                help for updatesparser
  -j, --json                Output in json format
  -o, --output string       Output file. Defaults to 'stdout'
  -p, --packages string     Package file list to filter updates modiying any of listed packages
  -s, --security            Match only security updates
  -t, --template string     Provides a custom update template file
```
