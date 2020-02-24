Licence Detector
================

Parses the output of `go list -m -json all` and attempts to detect licences of direct and indirect dependencies.

Usage
-----

```
licence-detector [FLAGS]

Flags:
  -depsOut string
    	Path to output the dependency list (default "dependencies.asciidoc")
  -depsTemplate string
    	Path to the dependency list template file (default "templates/dependencies.asciidoc.tmpl")
  -in string
    	Dependency list (output from go list -m -json all) (default "-")
  -includeIndirect
    	Include indirect dependencies
  -licenceData string
    	Path to the licence database (default "licence.db")
  -noticeOut string
    	Path to output the notice (default "NOTICE.txt")
  -noticeTemplate string
    	Path to the NOTICE template file (default "templates/NOTICE.txt.tmpl")
  -overrides string
    	Path to the file containing override directives
```

Adding Overrides
----------------

In some cases, the licence-detector will not be able to detect the licence type or infer the correct URL for a dependency. When there are issues with licences (no licence file or unknown licence type), the application will fail with an error message instructing the user to add an override to continue. The overrides file is a file containing newline-delimited JSON where each line contains a JSON object bearing the following format:

- `name`: Required. Module name to apply the override to.
- `licenceType`: Optional. Type of licence (Apache-2.0, ISC etc.). Provide a [SPDX](https://spdx.org/licenses/) identifier.
- `licenceTextFile`: Optional. Path to a file containing the licence text for this module. Path must be relative to the `overrides.json` file.
- `url`: Optional. URL to the dependency website.

Example overrides file:

```json
{"name": "github.com/bmizerany/perks", "licenceTextFile": "licences/github.com/bmizerany/perks/LICENCE"}
{"name": "github.com/dgryski/go-gk", "licenceType": "MIT"}
{"name": "github.com/russross/blackfriday/v2", "url": "https://gopkg.in/russross/blackfriday.v2"}
```

Updating the licence database
-----------------------------

The licence database file `licence.db` contains all the currently known licence types found in https://github.com/google/licenseclassifier/tree/master/licenses. In the rare case that entirely new licence types have been introduced to the codebase, follow the instructions at https://github.com/google/licenseclassifier to execute the `licence_serializer` tool.
