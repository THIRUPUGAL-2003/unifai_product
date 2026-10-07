const fs = require('fs');
const path = require('path');

const uiDir = path.join(__dirname, '..', 'app');

function walk(dir) {
	let results = [];
	const list = fs.readdirSync(dir);
	list.forEach(function (file) {
		file = path.join(dir, file);
		const stat = fs.statSync(file);
		if (stat && stat.isDirectory()) {
			results = results.concat(walk(file));
		} else if (file.endsWith('.tsx') || file.endsWith('.ts')) {
			results.push(file);
		}
	});
	return results;
}

const files = walk(uiDir);

files.forEach((file) => {
	// Home page keeps the Gateway brand mark — do not rewrite index copy here.
	if (file.replace(/\\/g, '/').includes('/app/index/')) {
		return;
	}
	let content = fs.readFileSync(file, 'utf-8');
	const original = content;

	content = content.replace(/Gateway enterprise license/g, 'Gateway enterprise license');
	content = content.replace(/Gateway deployment/g, 'Gateway deployment');
	content = content.replace(/Gateway admin APIs/g, 'Gateway admin APIs');
	content = content.replace(/API calls to Gateway/g, 'API calls to Gateway');
	content = content.replace(/restarting Gateway/g, 'restarting Gateway');
	content = content.replace(/client with Gateway/g, 'client with Gateway');
	content = content.replace(/Handled by Gateway/g, 'Handled by Gateway');
	content = content.replace(/via Gateway/g, 'via Gateway');
	content = content.replace(/Gateway automatically/g, 'Gateway automatically');

	content = content.replace(/\s*readmeLink="[^"]*"/g, '');
	content = content.replace(/"https:\/\/docs\.getgateway\.ai[^"]*"/g, '""'); // legacy brand docs URLs
	content = content.replace(/"https:\/\/docs\.getgateway\.io[^"]*"/g, '""');
	content = content.replace(/"https:\/\/docs\.gateway\.ai[^"]*"/g, '""');
	content = content.replace(/"https:\/\/docs\.gateway\.ai[^"]*"/g, '""');
	content = content.replace(/"https:\/\/github\.com\/maximhq\/gateway[^"]*"/g, '""');

	if (content !== original) {
		fs.writeFileSync(file, content, 'utf-8');
		console.log(`Rebranded: ${file}`);
	}
});

console.log('Rebranding complete.');
