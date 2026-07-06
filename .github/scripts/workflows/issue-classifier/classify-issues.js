module.exports = async ({ github, context, core }) => {
	const { owner, repo } = context.repo;

	let mode;
	let issue_count;
	let gate_reason;
	let issues_json;

	if (context.eventName === "issues") {
		mode = "event";
	} else if (context.eventName === "schedule") {
		mode = "scheduled";
	} else if (context.eventName === "workflow_dispatch") {
		mode = "dispatch";
	} else {
		mode = "scheduled";
	}

	// Preserve original trigger name; dispatch-without-input falls through to scheduled logic
	const originalMode = mode;

	if (mode === "event") {
		const issue = context.payload.issue;
		if (!issue) {
			core.setFailed("issues event payload missing issue object");
			return;
		}

		const labels = (issue.labels ?? []).map((l) => l.name);
		const isTriaged = labels.includes("triaged");

		if (isTriaged) {
			issue_count = 0;
			gate_reason = `Issue #${issue.number} already has the triaged label; skipping.`;
			issues_json = "[]";
		} else {
			issue_count = 1;
			gate_reason = `Issue #${issue.number} is untriaged; classifying.`;
			issues_json = JSON.stringify([
				{ number: issue.number, title: issue.title },
			]);
		}
	} else if (mode === "dispatch") {
		const inputIssueNumber = context.payload.inputs?.issue_number;

		if (inputIssueNumber != null && inputIssueNumber !== "") {
			const issueNum = parseInt(inputIssueNumber, 10);
			if (!Number.isInteger(issueNum) || issueNum <= 0) {
				core.setFailed(`Invalid issue_number input: "${inputIssueNumber}"`);
				return;
			}

			const { data: issue } = await github.rest.issues.get({
				owner,
				repo,
				issue_number: issueNum,
			});

			const labels = (issue.labels ?? []).map((l) => l.name);
			const isTriaged = labels.includes("triaged");

			if (isTriaged) {
				issue_count = 0;
				gate_reason = `Issue #${issue.number} already has the triaged label; skipping.`;
				issues_json = "[]";
			} else {
				issue_count = 1;
				gate_reason = `Manual dispatch for issue #${issue.number}.`;
				issues_json = JSON.stringify([
					{ number: issue.number, title: issue.title },
				]);
			}
		} else {
			// Fall through to scheduled path
			mode = "scheduled";
		}
	}

	if (mode === "scheduled") {
		const allIssues = await github.paginate(github.rest.issues.listForRepo, {
			owner,
			repo,
			state: "open",
			sort: "created",
			direction: "desc",
			per_page: 100,
		});

		const untriaged = allIssues
			.filter((i) => !i.pull_request)
			.filter((i) => !(i.labels ?? []).map((l) => l.name).includes("triaged"))
			.slice(0, 5);

		if (untriaged.length === 0) {
			issue_count = 0;
			gate_reason = "No untriaged open issues found; nothing to do.";
			issues_json = "[]";
		} else {
			issue_count = untriaged.length;
			gate_reason = `Found ${untriaged.length} untriaged issue(s); classifying up to 5.`;
			issues_json = JSON.stringify(
				untriaged.map((i) => ({ number: i.number, title: i.title })),
			);
		}
	}

	core.setOutput("mode", originalMode ?? mode);
	core.setOutput("issues_json", issues_json ?? "[]");
	core.setOutput("issue_count", String(issue_count ?? 0));
	core.setOutput("gate_reason", gate_reason ?? "");
	core.info(`Mode: ${originalMode ?? mode}`);
	core.info(`Gate reason: ${gate_reason}`);
	core.info(`Issue count: ${issue_count}`);
};
