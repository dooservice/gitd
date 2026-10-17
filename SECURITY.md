# Security policy

gitd holds the credentials that read private repositories and writes the code that runs
from its workspaces, so we treat every report seriously.

## Reporting a vulnerability

Do not open a public issue. Report it privately through GitHub:
[**Report a vulnerability**](https://github.com/dooservice/gitd/security/advisories/new),
in the *Security* tab of this repository, or by email to
[soporte@apiservicesac.com](mailto:soporte@apiservicesac.com) with «Security» in the
subject.

Include the version (`gitd --version`), what an attacker can do and from where (the
socket, a repository's contents, a submodule URL), and how to reproduce it.

We answer within **3 working days**, keep you informed while we fix it, and publish an
advisory once the fix is out, crediting you if you want. Please give us up to **90 days**
before disclosing.

## Scope

In scope: this service, its socket API and how it handles credentials, URLs and repository
contents. Out of scope: vulnerabilities in git itself; report those to the git project.

Only the latest release gets fixes.
