import sys
content = open('Makefile').read()
if 'install-vulncheck' not in content:
    content = content.replace('verify: verify-tdd verify-scenarios', 'install-vulncheck:\n\tgo install golang.org/x/vuln/cmd/govulncheck@latest\n\nverify: verify-tdd verify-scenarios install-vulncheck')
    open('Makefile', 'w').write(content)
