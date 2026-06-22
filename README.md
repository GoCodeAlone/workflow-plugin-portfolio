# workflow-plugin-portfolio

`workflow-plugin-portfolio` is a wfctl CLI plugin that generates a cross-repo
portfolio catalog: it walks the sibling git repositories under a workspace
root, dedupes by remote URL, captures per-repo git facts (remote, last commit,
uncommitted flag), and emits a portable catalog suitable for review or
further tooling. It is invoked by wfctl via the `capabilities.cliCommands`
plugin contract.

## Usage

```
wfctl portfolio scan    # walk sibling repos, emit a portfolio catalog
wfctl portfolio status  # report catalog freshness / staleness
```

The plugin binary is invoked by wfctl as `portfolio --wfctl-cli portfolio <subcommand>`;
end users call it through `wfctl portfolio ...`.
