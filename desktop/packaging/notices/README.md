# Supplementary package notices

The locked npm archives omit root license files for these two packages.
These supplementary notices are bundled by the desktop packager; unknown
missing notices still stop packaging. No dependency versions were changed.

- `react-remove-scroll-bar@2.3.8`: package.json selects MIT and names Anton
  Korzunov. Preserve the upstream project's LICENSE snapshot from commit
  `7301c160fda44cb8cf2b9fdfde61efad35736196`: https://raw.githubusercontent.com/theKashey/react-remove-scroll-bar/7301c160fda44cb8cf2b9fdfde61efad35736196/LICENSE
  SHA256: `a79aae0c0f21990d9d963bb3c5a79cdcea9a46f8523ba55c58d7fe776b6ebc84`. This is an upstream
  notice supplement, not a claim that the file was present in the npm archive.
- `dlv@1.1.3`: package.json selects MIT and names Jason Miller; README's license
  section also selects MIT but its former external license page no longer
  serves the license. The supplement uses standard MIT terms and the declared
  author attribution, without asserting an unknown copyright year. It is a
  reconstructed notice, not a copied upstream LICENSE file.

The collector also ignores an absent optional platform dependency (such as
macOS-only fsevents on this Linux build host), never a missing required package.
These notices are not a public-release legal-completeness approval.
