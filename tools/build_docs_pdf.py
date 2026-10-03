#!/usr/bin/env python3
"""
UnifAI / Raksha Enterprise Documentation PDF Generator
Converts markdown documentation files into professional, styled PDF documents using ReportLab.
"""

import os
import re
import sys
from reportlab.lib.pagesizes import letter
from reportlab.lib import colors
from reportlab.lib.units import inch
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle, PageBreak, KeepTogether, HRFlowable
)
from reportlab.pdfgen import canvas
from pypdf import PdfWriter

# Color Palette
PRIMARY_COLOR = colors.HexColor("#0f172a")      # Slate 900
SECONDARY_COLOR = colors.HexColor("#1e40af")    # Blue 800
ACCENT_COLOR = colors.HexColor("#2563eb")       # Blue 600
TEXT_COLOR = colors.HexColor("#1e293b")         # Slate 800
MUTED_TEXT = colors.HexColor("#64748b")         # Slate 500
BORDER_COLOR = colors.HexColor("#cbd5e1")       # Slate 300
CODE_BG = colors.HexColor("#f8fafc")            # Slate 50
CODE_BORDER = colors.HexColor("#e2e8f0")        # Slate 200
TABLE_HEADER_BG = colors.HexColor("#1e3a8a")    # Dark Blue
TABLE_ALT_ROW = colors.HexColor("#f8fafc")


class NumberedCanvas(canvas.Canvas):
    """
    Two-pass canvas to dynamically compute and draw 'Page X of Y' on all pages.
    """
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._saved_page_states = []

    def showPage(self):
        self._saved_page_states.append(dict(self.__dict__))
        self._startPage()

    def save(self):
        num_pages = len(self._saved_page_states)
        for state in self._saved_page_states:
            self.__dict__.update(state)
            self.draw_page_decorations(num_pages)
            super().showPage()
        super().save()

    def draw_page_decorations(self, page_count):
        self.saveState()
        width, height = letter

        # Running Header (pages > 1)
        if self._pageNumber > 1:
            self.setFont("Helvetica-Bold", 8)
            self.setFillColor(MUTED_TEXT)
            self.drawString(54, height - 36, "UNIFAI / RAKSHA ENTERPRISE AI PLATFORM")
            self.setFont("Helvetica", 8)
            self.drawRightString(width - 54, height - 36, "OFFICIAL SYSTEM MANUAL")

            self.setStrokeColor(BORDER_COLOR)
            self.setLineWidth(0.5)
            self.line(54, height - 42, width - 54, height - 42)

        # Running Footer (all pages)
        self.setStrokeColor(BORDER_COLOR)
        self.setLineWidth(0.5)
        self.line(54, 45, width - 54, 45)

        self.setFont("Helvetica", 8)
        self.setFillColor(MUTED_TEXT)
        self.drawString(54, 32, "Enterprise Confidential - For Internal & Authorized Use Only")
        page_str = f"Page {self._pageNumber} of {page_count}"
        self.drawRightString(width - 54, 32, page_str)

        self.restoreState()


def get_custom_styles():
    styles = getSampleStyleSheet()

    styles.add(ParagraphStyle(
        'DocTitle',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=20,
        leading=24,
        textColor=PRIMARY_COLOR,
        spaceAfter=6,
    ))

    styles.add(ParagraphStyle(
        'DocSubtitle',
        parent=styles['Normal'],
        fontName='Helvetica',
        fontSize=12,
        leading=16,
        textColor=SECONDARY_COLOR,
        spaceAfter=14,
    ))

    styles.add(ParagraphStyle(
        'SectionH1',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=14,
        leading=18,
        textColor=PRIMARY_COLOR,
        spaceBefore=14,
        spaceAfter=8,
        keepWithNext=True
    ))

    styles.add(ParagraphStyle(
        'SectionH2',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=11,
        leading=15,
        textColor=SECONDARY_COLOR,
        spaceBefore=10,
        spaceAfter=6,
        keepWithNext=True
    ))

    styles.add(ParagraphStyle(
        'SectionH3',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=10,
        leading=13,
        textColor=PRIMARY_COLOR,
        spaceBefore=8,
        spaceAfter=4,
        keepWithNext=True
    ))

    styles.add(ParagraphStyle(
        'BodyDark',
        parent=styles['Normal'],
        fontName='Helvetica',
        fontSize=9,
        leading=13,
        textColor=TEXT_COLOR,
        spaceAfter=6,
    ))

    styles.add(ParagraphStyle(
        'BulletItem',
        parent=styles['Normal'],
        fontName='Helvetica',
        fontSize=9,
        leading=13,
        textColor=TEXT_COLOR,
        leftIndent=15,
        firstLineIndent=-10,
        spaceAfter=4,
    ))

    styles.add(ParagraphStyle(
        'CodeBlockText',
        parent=styles['Normal'],
        fontName='Courier',
        fontSize=7.5,
        leading=10,
        textColor=colors.HexColor("#0f172a"),
    ))

    styles.add(ParagraphStyle(
        'TableHeader',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=8,
        leading=11,
        textColor=colors.white,
        alignment=0,
    ))

    styles.add(ParagraphStyle(
        'TableCell',
        parent=styles['Normal'],
        fontName='Helvetica',
        fontSize=7.5,
        leading=10.5,
        textColor=TEXT_COLOR,
    ))

    styles.add(ParagraphStyle(
        'CalloutText',
        parent=styles['Normal'],
        fontName='Helvetica-Oblique',
        fontSize=8.5,
        leading=12,
        textColor=colors.HexColor("#1e3a8a"),
    ))

    return styles


def escape_xml(text):
    text = text.replace('&', '&amp;')
    text = text.replace('<', '&lt;')
    text = text.replace('>', '&gt;')
    return text


def format_inline_markdown(text):
    """Formats bold, italic, and inline code while protecting xml entities."""
    # Temporarily tokenize inline code
    code_tokens = []
    def repl_code(m):
        code_tokens.append(m.group(1))
        return f"@@@CODE_TOKEN_{len(code_tokens)-1}@@@"

    text = re.sub(r'`([^`]+)`', repl_code, text)

    # Escape xml entities for normal text
    text = escape_xml(text)

    # Restore legitimate HTML break tags
    text = text.replace('&lt;br&gt;', '<br/>').replace('&lt;br/&gt;', '<br/>')

    # Replace bold and italic
    text = re.sub(r'\*\*(.+?)\*\*', r'<b>\1</b>', text)
    text = re.sub(r'__(.+?)__', r'<b>\1</b>', text)
    text = re.sub(r'(?<!\*)\*(?!\*)(.+?)(?<!\*)\*(?!\*)', r'<i>\1</i>', text)

    # Restore inline code with monospace styling
    for idx, token in enumerate(code_tokens):
        escaped_token = escape_xml(token)
        styled_code = f'<font face="Courier" color="#b91c1c"><b>{escaped_token}</b></font>'
        text = text.replace(f"@@@CODE_TOKEN_{idx}@@@", styled_code)

    # Clean up any raw LaTeX symbols and unicode punctuation unsupported by standard Type 1 fonts
    text = text.replace('$\\ge', '>= ').replace('$', '')
    text = text.replace('\u2014', ' - ').replace('\u2013', '-').replace('\u2018', "'").replace('\u2019', "'").replace('\u201c', '"').replace('\u201d', '"').replace('\u2192', '->')

    return text


def parse_markdown_to_flowables(md_content, styles):
    flowables = []
    lines = md_content.splitlines()
    i = 0
    total_lines = len(lines)

    in_code_block = False
    code_buffer = []

    in_table = False
    table_lines = []

    while i < total_lines:
        line = lines[i]

        # Handle Code Blocks (```)
        if line.strip().startswith('```'):
            if not in_code_block:
                in_code_block = True
                code_buffer = []
            else:
                in_code_block = False
                # Build styled code table
                code_text = "\n".join(code_buffer)
                escaped_code = escape_xml(code_text)
                p = Paragraph(f"<pre>{escaped_code}</pre>", styles['CodeBlockText'])
                t = Table([[p]], colWidths=[letter[0] - 108])
                t.setStyle(TableStyle([
                    ('BACKGROUND', (0,0), (-1,-1), CODE_BG),
                    ('BOX', (0,0), (-1,-1), 0.5, CODE_BORDER),
                    ('TOPPADDING', (0,0), (-1,-1), 6),
                    ('BOTTOMPADDING', (0,0), (-1,-1), 6),
                    ('LEFTPADDING', (0,0), (-1,-1), 8),
                    ('RIGHTPADDING', (0,0), (-1,-1), 8),
                ]))
                flowables.append(Spacer(1, 4))
                flowables.append(t)
                flowables.append(Spacer(1, 6))
                code_buffer = []
            i += 1
            continue

        if in_code_block:
            code_buffer.append(line)
            i += 1
            continue

        # Handle Markdown Tables (| col1 | col2 |)
        if line.strip().startswith('|') and line.strip().endswith('|'):
            table_lines.append(line.strip())
            i += 1
            # Check if table continues
            if i < total_lines and lines[i].strip().startswith('|') and lines[i].strip().endswith('|'):
                continue
            else:
                # Render accumulated table
                rendered_table = process_table(table_lines, styles)
                if rendered_table:
                    flowables.append(Spacer(1, 4))
                    flowables.append(rendered_table)
                    flowables.append(Spacer(1, 6))
                table_lines = []
                continue

        # Blank lines
        if not line.strip():
            i += 1
            continue

        # Page Break
        if line.strip().lower() in ['<!-- pagebreak -->', '<!-- page_break -->', '<!-- page-break -->', '<div style="page-break-after: always;"></div>', '<div style="page-break-after: always; break-after: page;"></div>', '\\newpage', '<!-- page break -->']:
            flowables.append(PageBreak())
            i += 1
            continue

        # Horizontal Rule
        if line.strip() in ['---', '***', '___']:
            flowables.append(Spacer(1, 4))
            flowables.append(HRFlowable(width="100%", thickness=0.5, color=BORDER_COLOR, spaceAfter=8, spaceBefore=4))
            i += 1
            continue

        # Headers
        if line.startswith('# '):
            text = format_inline_markdown(line[2:].strip())
            flowables.append(Paragraph(text, styles['DocTitle']))
            i += 1
            continue
        elif line.startswith('## '):
            text = format_inline_markdown(line[3:].strip())
            flowables.append(Paragraph(text, styles['SectionH1']))
            i += 1
            continue
        elif line.startswith('### '):
            text = format_inline_markdown(line[4:].strip())
            flowables.append(Paragraph(text, styles['SectionH2']))
            i += 1
            continue
        elif line.startswith('#### '):
            text = format_inline_markdown(line[5:].strip())
            flowables.append(Paragraph(text, styles['SectionH3']))
            i += 1
            continue

        # Blockquotes / Callouts (> ...)
        if line.startswith('> '):
            callout_text = format_inline_markdown(line[2:].strip())
            p = Paragraph(callout_text, styles['CalloutText'])
            t = Table([[p]], colWidths=[letter[0] - 108])
            t.setStyle(TableStyle([
                ('BACKGROUND', (0,0), (-1,-1), colors.HexColor("#eff6ff")),
                ('LINELEFT', (0,0), (0,-1), 3, ACCENT_COLOR),
                ('BOX', (0,0), (-1,-1), 0.5, colors.HexColor("#bfdbfe")),
                ('TOPPADDING', (0,0), (-1,-1), 6),
                ('BOTTOMPADDING', (0,0), (-1,-1), 6),
                ('LEFTPADDING', (0,0), (-1,-1), 10),
                ('RIGHTPADDING', (0,0), (-1,-1), 8),
            ]))
            flowables.append(Spacer(1, 4))
            flowables.append(t)
            flowables.append(Spacer(1, 6))
            i += 1
            continue

        # Unordered Lists (- or *)
        if re.match(r'^\s*[-*]\s+', line):
            content = re.sub(r'^\s*[-*]\s+', '', line)
            text = format_inline_markdown(content)
            bullet_char = "&bull;&nbsp;&nbsp;"
            flowables.append(Paragraph(f"{bullet_char}{text}", styles['BulletItem']))
            i += 1
            continue

        # Ordered Lists (1. 2. etc)
        if re.match(r'^\s*\d+\.\s+', line):
            num = re.match(r'^\s*(\d+\.)\s+', line).group(1)
            content = re.sub(r'^\s*\d+\.\s+', '', line)
            text = format_inline_markdown(content)
            flowables.append(Paragraph(f"<b>{num}</b>&nbsp;&nbsp;{text}", styles['BulletItem']))
            i += 1
            continue

        # Regular Paragraph
        text = format_inline_markdown(line.strip())
        flowables.append(Paragraph(text, styles['BodyDark']))
        i += 1

    return flowables


def process_table(table_lines, styles):
    if len(table_lines) < 2:
        return None

    # Parse rows
    raw_rows = []
    for line in table_lines:
        cells = [c.strip() for c in line.strip('|').split('|')]
        raw_rows.append(cells)

    # Determine if second row is separator (|---|---|)
    if len(raw_rows) >= 2 and all(re.match(r'^-+$', c) for c in raw_rows[1]):
        header = raw_rows[0]
        data_rows = raw_rows[2:]
    else:
        header = raw_rows[0]
        data_rows = raw_rows[1:]

    num_cols = len(header)
    if num_cols == 0:
        return None

    # Calculate column widths to fit page exactly (printable width = letter[0] - 108 = 504 pt)
    available_width = letter[0] - 108
    col_width = available_width / float(num_cols)
    col_widths = [col_width] * num_cols

    # Build Table data
    table_data = []
    # Header
    header_row = [Paragraph(format_inline_markdown(h), styles['TableHeader']) for h in header]
    table_data.append(header_row)

    # Data
    for r_idx, row in enumerate(data_rows):
        # Pad row if fewer cols
        padded_row = row + [''] * (num_cols - len(row))
        data_row = [Paragraph(format_inline_markdown(c), styles['TableCell']) for c in padded_row[:num_cols]]
        table_data.append(data_row)

    t = Table(table_data, colWidths=col_widths, repeatRows=1)
    t_style = [
        ('BACKGROUND', (0,0), (-1,0), TABLE_HEADER_BG),
        ('GRID', (0,0), (-1,-1), 0.5, BORDER_COLOR),
        ('TOPPADDING', (0,0), (-1,-1), 4),
        ('BOTTOMPADDING', (0,0), (-1,-1), 4),
        ('LEFTPADDING', (0,0), (-1,-1), 5),
        ('RIGHTPADDING', (0,0), (-1,-1), 5),
        ('VALIGN', (0,0), (-1,-1), 'TOP'),
    ]

    # Alternating row background
    for row_idx in range(1, len(table_data)):
        if row_idx % 2 == 0:
            t_style.append(('BACKGROUND', (0, row_idx), (-1, row_idx), TABLE_ALT_ROW))

    t.setStyle(TableStyle(t_style))
    return t


def convert_markdown_file_to_pdf(input_md_path, output_pdf_path):
    print(f"[*] Processing: {input_md_path} -> {output_pdf_path}")
    with open(input_md_path, 'r', encoding='utf-8') as f:
        content = f.read()

    doc = SimpleDocTemplate(
        output_pdf_path,
        pagesize=letter,
        leftMargin=54,
        rightMargin=54,
        topMargin=54,
        bottomMargin=54
    )

    styles = get_custom_styles()
    flowables = parse_markdown_to_flowables(content, styles)

    # Build document with custom canvas
    doc.build(flowables, canvasmaker=NumberedCanvas)
    print(f"  [OK] Successfully generated: {output_pdf_path} ({os.path.getsize(output_pdf_path)} bytes)")


def merge_all_pdfs(pdf_list, output_master_pdf):
    print(f"[*] Merging all documents into Master PDF: {output_master_pdf}")
    writer = PdfWriter()
    for pdf_file in pdf_list:
        if os.path.exists(pdf_file):
            print(f"  [+] Adding: {os.path.basename(pdf_file)}")
            writer.append(pdf_file)
    writer.write(output_master_pdf)
    writer.close()
    print(f"  [OK] Master PDF created: {output_master_pdf} ({os.path.getsize(output_master_pdf)} bytes)")


def main():
    base_dir = r"d:\unifai_project\pdf"
    docs_pdf_dir = r"d:\unifai_project\docs\pdf"

    os.makedirs(base_dir, exist_ok=True)
    os.makedirs(docs_pdf_dir, exist_ok=True)

    doc_files = [
        ("00_DOCUMENTATION_OVERVIEW.md", "00_Documentation_Overview.pdf"),
        ("01_TECHNICAL_DOCUMENTATION.md", "01_Technical_Documentation.pdf"),
        ("02_SERVER_IMPLEMENTATION_GUIDE.md", "02_Server_Implementation_Guide.pdf"),
        ("03_ADMIN_CONSOLE_GUIDE.md", "03_Admin_Console_Guide.pdf"),
        ("04_USER_GUIDE.md", "04_User_Guide.pdf"),
        ("05_DATABASE_CONFIGURATION_GUIDE.md", "05_Database_Configuration_Guide.pdf"),
        ("06_DOCKER_AND_SSL_CONFIGURATION_GUIDE.md", "06_Docker_and_SSL_Configuration_Guide.pdf"),
        ("Raksha_Technical_Documentation.md", "Raksha_Technical_Documentation.pdf"),
    ]

    generated_pdfs = []

    for md_name, pdf_name in doc_files:
        md_path = os.path.join(base_dir, md_name)
        pdf_path = os.path.join(base_dir, pdf_name)
        if os.path.exists(md_path):
            convert_markdown_file_to_pdf(md_path, pdf_path)
            generated_pdfs.append(pdf_path)

            # Copy to docs/pdf as well for redundancy
            dest_pdf = os.path.join(docs_pdf_dir, pdf_name)
            dest_md = os.path.join(docs_pdf_dir, md_name)
            with open(pdf_path, 'rb') as src, open(dest_pdf, 'wb') as dst:
                dst.write(src.read())
            with open(md_path, 'rb') as src, open(dest_md, 'wb') as dst:
                dst.write(src.read())

    # Build Master Combined PDF
    master_pdf = os.path.join(base_dir, "UnifAI_Enterprise_Complete_Documentation.pdf")
    merge_all_pdfs(generated_pdfs, master_pdf)

    # Mirror Master PDF to docs/pdf
    master_dest = os.path.join(docs_pdf_dir, "UnifAI_Enterprise_Complete_Documentation.pdf")
    with open(master_pdf, 'rb') as src, open(master_dest, 'wb') as dst:
        dst.write(src.read())

    print("\n[SUCCESS] ALL DOCUMENTATION PDFS GENERATED SUCCESSFULLY!")


if __name__ == "__main__":
    main()
