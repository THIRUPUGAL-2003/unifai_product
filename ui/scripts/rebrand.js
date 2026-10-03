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

    content = content.replace(/Raksha enterprise license/g, 'Raksha enterprise license');
    content = content.replace(/Raksha deployment/g, 'Raksha deployment');
    content = content.replace(/Raksha admin APIs/g, 'Raksha admin APIs');
    content = content.replace(/API calls to Raksha/g, 'API calls to Raksha');
    content = content.replace(/restarting Raksha/g, 'restarting Raksha');
    content = content.replace(/client with Raksha/g, 'client with Raksha');
    content = content.replace(/Handled by Raksha/g, 'Handled by Raksha');
    content = content.replace(/via Raksha/g, 'via Raksha');
    content = content.replace(/Raksha automatically/g, 'Raksha automatically');

    content = content.replace(/\s*readmeLink="[^"]*"/g, '');
    content = content.replace(/"https:\/\/docs\.getraksha\.ai[^"]*"/g, '""');
    content = content.replace(/"https:\/\/docs\.getraksha\.io[^"]*"/g, '""');
    content = content.replace(/"https:\/\/github\.com\/maximhq\/raksha[^"]*"/g, '""');

    if (content !== original) {
        fs.writeFileSync(file, content, 'utf-8');
        console.log(`Rebranded: ${file}`);
    }
});

console.log('Rebranding complete.');
