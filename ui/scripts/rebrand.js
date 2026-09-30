const fs = require('fs');
const path = require('path');

const uiDir = path.join(__dirname, '..', 'app');

function walk(dir) {
    let results = [];
    const list = fs.readdirSync(dir);
    list.forEach(function(file) {
        file = path.join(dir, file);
        const stat = fs.statSync(file);
        if (stat && stat.isDirectory()) {
            results = results.concat(walk(file));
        } else {
            if (file.endsWith('.tsx') || file.endsWith('.ts')) {
                results.push(file);
            }
        }
    });
    return results;
}

const files = walk(uiDir);

files.forEach(file => {
    let content = fs.readFileSync(file, 'utf-8');
    let original = content;

    content = content.replace(/UnifAI enterprise license/g, 'UnifAI enterprise license');
    content = content.replace(/UnifAI deployment/g, 'UnifAI deployment');
    content = content.replace(/UnifAI admin APIs/g, 'UnifAI admin APIs');
    content = content.replace(/API calls to UnifAI/g, 'API calls to UnifAI');
    content = content.replace(/restarting UnifAI/g, 'restarting UnifAI');
    content = content.replace(/client with UnifAI/g, 'client with UnifAI');
    content = content.replace(/Handled by UnifAI/g, 'Handled by UnifAI');
    content = content.replace(/via UnifAI/g, 'via UnifAI');
    content = content.replace(/UnifAI automatically/g, 'UnifAI automatically');

    content = content.replace(/\s*readmeLink="[^"]*"/g, '');
    content = content.replace(/"https:\/\/docs\.getunifai\.ai[^"]*"/g, '""');
    content = content.replace(/"https:\/\/docs\.getunifai\.io[^"]*"/g, '""');
    content = content.replace(/"https:\/\/github\.com\/maximhq\/unifai[^"]*"/g, '""');

    if (content !== original) {
        fs.writeFileSync(file, content, 'utf-8');
        console.log(`Rebranded: ${file}`);
    }
});

console.log('Rebranding complete.');
