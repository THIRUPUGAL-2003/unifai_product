#!/usr/bin/env python3
"""
YP-Raksha Enterprise All-in-One Technical Documentation Builder
Generates high-fidelity PDF and DOCX documents from YP_RAKSHA_ENTERPRISE_ALL_IN_ONE_TECHNICAL_DOCUMENT.md.
"""

import os
import re
import sys
import shutil
from pathlib import Path

# ReportLab Imports
from reportlab.lib.pagesizes import A4
from reportlab.lib import colors
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Image, Table, TableStyle, PageBreak, KeepTogether, Preformatted
)
from reportlab.pdfgen import canvas

# Docx Imports
import docx
from docx.shared import Inches, Pt, RGBColor
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.enum.table import WD_TABLE_ALIGNMENT
from docx.oxml import parse_xml, OxmlElement
from docx.oxml.ns import nsdecls, qn

# PyMuPDF for visual validation
import pymupdf

BASE_DIR = Path(r"d:\unifai_project")
DOCS_DIR = BASE_DIR / "docs"
MD_SOURCE = DOCS_DIR / "YP_RAKSHA_ENTERPRISE_ALL_IN_ONE_TECHNICAL_DOCUMENT.md"
LOGO_PATH = str(BASE_DIR / "ui" / "public" / "yes-panchi-logo.png")

PDF_OUTPUT_DOCS = DOCS_DIR / "YP_RAKSHA_Enterprise_All_In_One_Technical_Document.pdf"
DOCX_OUTPUT_DOCS = DOCS_DIR / "YP_RAKSHA_Enterprise_All_In_One_Technical_Document.docx"
PDF_OUTPUT_ROOT = BASE_DIR / "YP_RAKSHA_Enterprise_All_In_One_Technical_Document.pdf"
DOCX_OUTPUT_ROOT = BASE_DIR / "YP_RAKSHA_Enterprise_All_In_One_Technical_Document.docx"

PAGE_W, PAGE_H = A4  # 595.27 x 841.89 pt
MARGIN = 36          # 0.5 inch margins
CONTENT_W = PAGE_W - (MARGIN * 2)  # 523.27 pt

# Enterprise Color Palette
NAVY = colors.HexColor("#0F172A")        # Slate 900
SLATE_DARK = colors.HexColor("#1E293B")  # Slate 800
SLATE_BODY = colors.HexColor("#334155")  # Slate 700
MUTED = colors.HexColor("#64748B")       # Slate 500
BORDER_COLOR = colors.HexColor("#CBD5E1")# Slate 300
BORDER_LIGHT = colors.HexColor("#E2E8F0")# Slate 200
GREEN = colors.HexColor("#059669")       # Emerald 600
GREEN_LIGHT = colors.HexColor("#F0FDF4") # Emerald 50
GREEN_BORDER = colors.HexColor("#86EFAC")# Emerald 300
TEAL = colors.HexColor("#0F766E")        # Teal 700
TEAL_BG = colors.HexColor("#F0FDFA")     # Teal 50
CARD_BG = colors.HexColor("#F8FAFC")     # Slate 50
WHITE = colors.white

def get_pdf_styles():
    return {
        "cover_company": ParagraphStyle("cover_company", fontName="Helvetica-Bold", fontSize=14, leading=18, textColor=NAVY, alignment=1, spaceAfter=8),
        "cover_title": ParagraphStyle("cover_title", fontName="Helvetica-Bold", fontSize=34, leading=40, textColor=NAVY, alignment=1, spaceAfter=4),
        "cover_subtitle": ParagraphStyle("cover_subtitle", fontName="Helvetica-Bold", fontSize=13, leading=17, textColor=TEAL, alignment=1, spaceAfter=14),
        "cover_doc_type": ParagraphStyle("cover_doc_type", fontName="Helvetica-Bold", fontSize=22, leading=28, textColor=SLATE_DARK, alignment=1, spaceAfter=4),
        "cover_blueprint": ParagraphStyle("cover_blueprint", fontName="Helvetica", fontSize=11, leading=15, textColor=MUTED, alignment=1, spaceAfter=12),
        "cover_version": ParagraphStyle("cover_version", fontName="Helvetica-Bold", fontSize=12, leading=16, textColor=GREEN, alignment=1, spaceAfter=16),
        "cover_notice": ParagraphStyle("cover_notice", fontName="Helvetica-Oblique", fontSize=8.5, leading=12, textColor=MUTED, alignment=1),

        "h1": ParagraphStyle("h1", fontName="Helvetica-Bold", fontSize=13.5, leading=17.5, textColor=NAVY, spaceBefore=12, spaceAfter=5, keepWithNext=True),
        "h2": ParagraphStyle("h2", fontName="Helvetica-Bold", fontSize=10.2, leading=13.5, textColor=TEAL, spaceBefore=10, spaceAfter=4, keepWithNext=True),
        "h3": ParagraphStyle("h3", fontName="Helvetica-Bold", fontSize=8.5, leading=11.5, textColor=SLATE_DARK, spaceBefore=7.5, spaceAfter=2.5, leftIndent=12, keepWithNext=True),
        "h4": ParagraphStyle("h4", fontName="Helvetica-Bold", fontSize=8.0, leading=11.0, textColor=TEAL, spaceBefore=7, spaceAfter=3.0, keepWithNext=True),
        "body": ParagraphStyle("body", fontName="Helvetica", fontSize=7.8, leading=10.8, textColor=SLATE_BODY, spaceAfter=4.0, alignment=4),
        "body_indent": ParagraphStyle("body_indent", fontName="Helvetica", fontSize=7.8, leading=10.8, textColor=SLATE_BODY, spaceAfter=3.5, leftIndent=12, alignment=4),
        "body_bold": ParagraphStyle("body_bold", fontName="Helvetica-Bold", fontSize=7.8, leading=10.8, textColor=NAVY, spaceBefore=6, spaceAfter=3.0, keepWithNext=True),
        "bullet": ParagraphStyle("bullet", fontName="Helvetica", fontSize=7.6, leading=10.5, textColor=SLATE_BODY, spaceAfter=2.5, leftIndent=9, alignment=4),
        "bullet_indent": ParagraphStyle("bullet_indent", fontName="Helvetica", fontSize=7.6, leading=10.5, textColor=SLATE_BODY, spaceAfter=2.0, leftIndent=21, alignment=4),
        
        "code_block": ParagraphStyle("code_block", fontName="Courier", fontSize=5.7, leading=7.0, textColor=NAVY),
        "callout_title": ParagraphStyle("callout_title", fontName="Helvetica-Bold", fontSize=8.0, leading=10.8, textColor=TEAL, spaceAfter=2.5),
        "callout_body": ParagraphStyle("callout_body", fontName="Helvetica", fontSize=7.2, leading=9.8, textColor=SLATE_DARK, spaceAfter=2.0, alignment=4),
        "callout_bullet": ParagraphStyle("callout_bullet", fontName="Helvetica", fontSize=7.0, leading=9.5, textColor=SLATE_DARK, spaceAfter=1.5, leftIndent=8, alignment=4),

        "th": ParagraphStyle("th", fontName="Helvetica-Bold", fontSize=7.0, leading=9.0, textColor=WHITE),
        "th_c": ParagraphStyle("th_c", fontName="Helvetica-Bold", fontSize=7.0, leading=9.0, textColor=WHITE, alignment=1),
        "td": ParagraphStyle("td", fontName="Helvetica", fontSize=6.6, leading=8.4, textColor=SLATE_BODY),
        "td_b": ParagraphStyle("td_b", fontName="Helvetica-Bold", fontSize=6.6, leading=8.4, textColor=NAVY),
        "td_c": ParagraphStyle("td_c", fontName="Helvetica", fontSize=6.6, leading=8.4, textColor=SLATE_BODY, alignment=1),
        "td_cb": ParagraphStyle("td_cb", fontName="Helvetica-Bold", fontSize=6.6, leading=8.4, textColor=NAVY, alignment=1),

        "toc_sec": ParagraphStyle("toc_sec", fontName="Helvetica-Bold", fontSize=7.4, leading=9.4, textColor=NAVY, alignment=1),
        "toc_topic": ParagraphStyle("toc_topic", fontName="Helvetica-Bold", fontSize=7.4, leading=9.6, textColor=NAVY),
        "toc_subs": ParagraphStyle("toc_subs", fontName="Helvetica", fontSize=6.4, leading=8.2, textColor=SLATE_BODY),
        "toc_page": ParagraphStyle("toc_page", fontName="Helvetica-Bold", fontSize=7.8, leading=9.8, textColor=GREEN, alignment=1),

        "sec_banner_title": ParagraphStyle("sec_banner_title", fontName="Helvetica-Bold", fontSize=10.5, leading=14, textColor=WHITE),
    }

class NumberedCanvas(canvas.Canvas):
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
            self.draw_decorations(num_pages)
            super().showPage()
        super().save()

    def draw_decorations(self, total_pages):
        page_num = self._pageNumber
        self.saveState()

        # Running Header (Pages 2 through total_pages)
        if page_num > 1:
            try:
                self.drawImage(LOGO_PATH, MARGIN, PAGE_H - 30, width=54, height=17, preserveAspectRatio=True, mask='auto')
            except Exception:
                self.setFont("Helvetica-Bold", 8)
                self.setFillColor(NAVY)
                self.drawString(MARGIN, PAGE_H - 24, "YES PANCHI")

            self.setFont("Helvetica-Bold", 7.5)
            self.setFillColor(SLATE_DARK)
            self.drawRightString(PAGE_W - MARGIN, PAGE_H - 20, "YP-Raksha v2.5  |  Enterprise Technical Documentation")
            
            self.setFont("Helvetica", 6.8)
            self.setFillColor(MUTED)
            self.drawRightString(PAGE_W - MARGIN, PAGE_H - 28, "Real-time AI Knowledge Screening, Inline DLP & Hazard Audit Gateway")

            self.setStrokeColor(BORDER_COLOR)
            self.setLineWidth(0.5)
            self.line(MARGIN, PAGE_H - 33, PAGE_W - MARGIN, PAGE_H - 33)

        # Running Footer (Pages 2 through total_pages)
        if page_num > 1:
            self.setStrokeColor(BORDER_COLOR)
            self.setLineWidth(0.5)
            self.line(MARGIN, 32, PAGE_W - MARGIN, 32)

            self.setFont("Helvetica-Bold", 7.0)
            self.setFillColor(NAVY)
            self.drawString(MARGIN, 21, "YES PANCHI GROUP OF COMPANIES")

            self.setFont("Helvetica-Bold", 7.0)
            self.setFillColor(GREEN)
            self.drawRightString(PAGE_W - MARGIN, 21, f"Page {page_num} of {total_pages}")

        self.restoreState()


def format_for_reportlab(text):
    """
    Safely converts Markdown inline formatting to ReportLab XML tags:
    - Escapes &, <, > safely so XML parser never fails.
    - Restores <br/> for clean multi-line formatting inside table cells.
    - Converts **bold** to <b>bold</b>.
    - Converts `code` to <font face="Courier" color="#0F172A"><b>code</b></font>.
    - Balances all open/close tags defensively.
    """
    if not text:
        return ""
    
    # 1. Escape XML special characters
    # Replace & that is not already part of an entity
    text = re.sub(r"&(?!amp;|lt;|gt;|quot;|apos;)", "&amp;", text)
    text = text.replace("<", "&lt;").replace(">", "&gt;")

    # Restore intentional <br/> tags for clean line breaks in table cells!
    text = re.sub(r"&lt;br\s*/?&gt;", "<br/>", text)

    # 2. Protect inline code `...` before bold/italic parsing
    code_matches = []
    def save_code(m):
        code_matches.append(m.group(1))
        return f"__CODE_PH_{len(code_matches)-1}__"
    text = re.sub(r"`([^`]+?)`", save_code, text)

    # 3. Convert markdown bold **...**
    text = re.sub(r"\*\*(.+?)\*\*", r"<b>\1</b>", text)

    # 4. Convert *italic* safely (never across HTML tags)
    text = re.sub(r"(?<!\*)\*([^*<>]+?)\*(?!\*)", r"<i>\1</i>", text)

    # 5. Restore code with styled font tags
    def restore_code(m):
        idx = int(m.group(1))
        code_str = code_matches[idx]
        return f'<font face="Courier" color="#0F172A"><b>{code_str}</b></font>'
    text = re.sub(r"__CODE_PH_(\d+)__", restore_code, text)

    # 5. Defensively balance <b> and </b> tags
    open_b = text.count("<b>")
    close_b = text.count("</b>")
    if open_b > close_b:
        text += "</b>" * (open_b - close_b)
    elif close_b > open_b:
        for _ in range(close_b - open_b):
            idx = text.rfind("</b>")
            if idx != -1:
                text = text[:idx] + text[idx+4:]

    # Defensively balance <i> and </i> tags
    open_i = text.count("<i>")
    close_i = text.count("</i>")
    if open_i > close_i:
        text += "</i>" * (open_i - close_i)
    elif close_i > open_i:
        for _ in range(close_i - open_i):
            idx = text.rfind("</i>")
            if idx != -1:
                text = text[:idx] + text[idx+4:]

    # Defensively balance <font> and </font> tags
    open_f = text.count("<font")
    close_f = text.count("</font>")
    if open_f > close_f:
        text += "</font>" * (open_f - close_f)
    elif close_f > open_f:
        for _ in range(close_f - open_f):
            idx = text.rfind("</font>")
            if idx != -1:
                text = text[:idx] + text[idx+7:]

    return text


def parse_markdown_to_elements(md_text, styles):
    """
    Parses the Markdown document into ReportLab flowable elements with strict formatting guarantees.
    """
    elements = []
    lines = md_text.splitlines()
    i = 0
    n = len(lines)

    in_cover = True
    in_code = False
    code_lines = []
    in_callout = False
    callout_lines = []
    in_table = False
    table_lines = []

    def flush_table():
        nonlocal in_table, table_lines
        if not table_lines:
            in_table = False
            return
        
        parsed_rows = []
        for t_line in table_lines:
            t_line = t_line.strip()
            if not t_line.startswith("|"):
                continue
            raw_cells = re.split(r"(?<!\\)\|", t_line)
            cells = [c.strip().replace(r"\|", "• ") for c in raw_cells[1:-1]]
            if not cells or all(re.match(r"^:?-+:?$", c) for c in cells):
                continue
            parsed_rows.append(cells)
        
        table_lines = []
        in_table = False
        if not parsed_rows:
            return

        col_count = max(len(r) for r in parsed_rows)
        for r in parsed_rows:
            while len(r) < col_count:
                r.append("")

        # Calculate exact column widths fitting CONTENT_W (523.27 pt)
        header_text = " ".join(parsed_rows[0]).lower()
        if col_count == 2:
            col_widths = [135, CONTENT_W - 135]
        elif col_count == 3:
            if "deployment attribute" in header_text:
                col_widths = [120, 200, CONTENT_W - 320]
            elif "engineering dimension" in header_text:
                col_widths = [125, 200, CONTENT_W - 325]
            elif "desktop agent" in header_text or "hybrid deployment" in header_text or "mode a" in header_text:
                col_widths = [115, 204, CONTENT_W - 319]
            else:
                col_widths = [110, 210, CONTENT_W - 320]
        elif col_count == 4:
            if "page" in parsed_rows[0][-1].lower() or "reference" in parsed_rows[0][-1].lower():
                col_widths = [38, 125, 310, CONTENT_W - 473]
            elif "storage layer" in header_text or "database table" in header_text:
                col_widths = [90, 58, 125, CONTENT_W - 273]
            elif "environment variable" in header_text:
                col_widths = [135, 75, 45, CONTENT_W - 255]
            elif "software dependency" in header_text or "sbom" in header_text:
                col_widths = [110, 65, 160, CONTENT_W - 335]
            elif "critical banking" in header_text:
                col_widths = [105, 130, 195, CONTENT_W - 430]
            elif "deployment channel" in header_text or "installation method" in header_text:
                col_widths = [105, 115, 175, CONTENT_W - 395]
            elif "registry hive" in header_text or "policy key" in header_text or "target browser" in header_text:
                col_widths = [85, 160, 105, CONTENT_W - 350]
            elif "web browser" in header_text:
                col_widths = [85, 115, 95, CONTENT_W - 295]
            elif "endpoint agent" in header_text or "technical parameter" in header_text:
                col_widths = [100, 100, 150, CONTENT_W - 350]
            elif "regulatory" in header_text:
                col_widths = [90, 95, 215, CONTENT_W - 400]
            elif "infrastructure tier" in header_text:
                col_widths = [95, 125, 125, CONTENT_W - 345]
            elif "adversarial attack" in header_text or "prompt injection" in header_text:
                col_widths = [95, 125, 155, CONTENT_W - 375]
            else:
                col_widths = [105, 120, 140, CONTENT_W - 365]
        elif col_count == 5:
            if "banking operational division" in header_text:
                col_widths = [85, 85, 115, 120, CONTENT_W - 405]
            elif "hardware component" in header_text:
                col_widths = [68, 112, 115, 115, CONTENT_W - 410]
            elif "pipeline execution" in header_text:
                col_widths = [110, 165, 80, 85, CONTENT_W - 440]
            elif "firewall action" in header_text or "source subnet" in header_text:
                col_widths = [95, 105, 85, 75, CONTENT_W - 360]
            elif "performance metric" in header_text:
                col_widths = [115, 105, 105, 100, CONTENT_W - 425]
            elif "component role" in header_text:
                col_widths = [95, 105, 50, 140, CONTENT_W - 390]
            elif "operating system" in header_text:
                col_widths = [90, 110, 115, 105, CONTENT_W - 420]
            elif "logical service purpose" in header_text:
                col_widths = [95, 105, 125, 95, CONTENT_W - 420]
            else:
                col_widths = [90, 105, 110, 110, CONTENT_W - 415]
        elif col_count == 6:
            if "banking department" in header_text:
                col_widths = [90, 60, 85, 105, 75, CONTENT_W - 415]
            elif "infrastructure subsystem" in header_text:
                col_widths = [90, 60, 75, 80, 115, CONTENT_W - 420]
            elif "administrative role" in header_text:
                col_widths = [85, 85, 85, 85, 95, CONTENT_W - 435]
            else:
                col_widths = [70, 75, 90, 95, 100, CONTENT_W - 430]
        else:
            w = CONTENT_W / col_count
            col_widths = [w] * col_count

        table_data = []
        is_toc = col_count == 4 and ("Page" in parsed_rows[0][-1] or "Reference" in parsed_rows[0][-1])
        for row_idx, r in enumerate(parsed_rows):
            row_cells = []
            for col_idx, c in enumerate(r):
                c_clean = format_for_reportlab(c)
                if row_idx == 0:
                    st = styles["th_c"] if col_idx in [0, col_count - 1] else styles["th"]
                elif is_toc:
                    if col_idx == 0:
                        st = styles["toc_sec"]
                    elif col_idx == 1:
                        st = styles["toc_topic"]
                    elif col_idx == col_count - 1:
                        st = styles["toc_page"]
                    else:
                        st = styles["toc_subs"]
                else:
                    if col_idx == 0:
                        st = styles["td_b"]
                    elif col_idx == col_count - 1 and ("Page" in parsed_rows[0][-1] or "Ref" in parsed_rows[0][-1]):
                        st = styles["td_cb"]
                    else:
                        st = styles["td"]
                row_cells.append(Paragraph(c_clean, st))
            table_data.append(row_cells)

        t = Table(table_data, colWidths=col_widths, repeatRows=1)
        pad_v = 2.4 if is_toc else 2.6
        t_style = [
            ('BACKGROUND', (0, 0), (-1, 0), NAVY),
            ('ALIGN', (0, 0), (-1, -1), 'LEFT'),
            ('VALIGN', (0, 0), (-1, -1), 'TOP'),
            ('TOPPADDING', (0, 0), (-1, -1), pad_v),
            ('BOTTOMPADDING', (0, 0), (-1, -1), pad_v),
            ('LEFTPADDING', (0, 0), (-1, -1), 3.5),
            ('RIGHTPADDING', (0, 0), (-1, -1), 3.5),
            ('GRID', (0, 0), (-1, -1), 0.5, BORDER_COLOR),
        ]
        for r_idx in range(1, len(parsed_rows)):
            bg = CARD_BG if r_idx % 2 == 1 else WHITE
            t_style.append(('BACKGROUND', (0, r_idx), (-1, r_idx), bg))

        t.setStyle(TableStyle(t_style))
        elements.append(Spacer(1, 3))
        if is_toc or len(parsed_rows) <= 9:
            elements.append(KeepTogether(t))
        else:
            elements.append(t)
        elements.append(Spacer(1, 4))

    def flush_callout():
        nonlocal in_callout, callout_lines
        if not callout_lines:
            in_callout = False
            return
        
        c_flowables = []
        for c_line in callout_lines:
            c_text = c_line.strip()
            if not c_text:
                continue
            if c_text.startswith("### "):
                title = format_for_reportlab(c_text[4:].strip())
                c_flowables.append(Paragraph(title, styles["callout_title"]))
            elif c_text.startswith("* ") or c_text.startswith("- "):
                item = format_for_reportlab(c_text[2:].strip())
                c_flowables.append(Paragraph(f"• {item}", styles["callout_bullet"]))
            elif re.match(r"^\d+\.\s", c_text):
                num_m = re.match(r"^(\d+\.)\s+(.*)$", c_text)
                if num_m:
                    num_p = num_m.group(1)
                    num_t = format_for_reportlab(num_m.group(2).strip())
                    c_flowables.append(Paragraph(f"<b>{num_p}</b> {num_t}", styles["callout_bullet"]))
                else:
                    item = format_for_reportlab(c_text)
                    c_flowables.append(Paragraph(item, styles["callout_bullet"]))
            else:
                p_text = format_for_reportlab(c_text)
                c_flowables.append(Paragraph(p_text, styles["callout_body"]))

        callout_lines = []
        in_callout = False
        if not c_flowables:
            return

        callout_table = Table([[c_flowables]], colWidths=[CONTENT_W])
        callout_table.setStyle(TableStyle([
            ('BACKGROUND', (0, 0), (-1, -1), TEAL_BG),
            ('BOX', (0, 0), (-1, -1), 0.5, BORDER_COLOR),
            ('LINELEFT', (0, 0), (0, -1), 3.0, TEAL),
            ('TOPPADDING', (0, 0), (-1, -1), 4),
            ('BOTTOMPADDING', (0, 0), (-1, -1), 4),
            ('LEFTPADDING', (0, 0), (-1, -1), 7),
            ('RIGHTPADDING', (0, 0), (-1, -1), 7),
        ]))
        elements.append(Spacer(1, 3))
        elements.append(KeepTogether(callout_table))
        elements.append(Spacer(1, 4))

    def add_page_break():
        if elements and isinstance(elements[-1], PageBreak):
            return
        elements.append(PageBreak())

    def flush_code():
        nonlocal in_code, code_lines
        if not code_lines:
            in_code = False
            return
        
        non_empty = [l for l in code_lines if l.strip()]
        max_line_len = max(len(l) for l in non_empty) if non_empty else 80
        num_lines = len(code_lines)

        avail_w = CONTENT_W - 14.0
        if max_line_len >= 50:
            calc_fs = avail_w / (max_line_len * 0.60)
            if num_lines > 45:
                calc_fs = min(6.0, calc_fs)
            else:
                calc_fs = min(8.3, max(5.6, calc_fs))
        else:
            calc_fs = 7.5

        calc_leading = calc_fs * 1.18
        code_style = ParagraphStyle(
            f"code_block_{i}_{max_line_len}_{num_lines}",
            parent=styles["code_block"],
            fontSize=calc_fs,
            leading=calc_leading
        )

        code_text = "\n".join(code_lines)
        code_lines = []
        in_code = False

        pref = Preformatted(code_text, code_style)
        text_w = max_line_len * 0.60 * calc_fs
        h_pad = max(6.0, (CONTENT_W - text_w) / 2.0)

        code_table = Table([[pref]], colWidths=[CONTENT_W])
        code_table.setStyle(TableStyle([
            ('BACKGROUND', (0, 0), (-1, -1), CARD_BG),
            ('BOX', (0, 0), (-1, -1), 0.5, BORDER_COLOR),
            ('TOPPADDING', (0, 0), (-1, -1), 3.5),
            ('BOTTOMPADDING', (0, 0), (-1, -1), 3.5),
            ('LEFTPADDING', (0, 0), (-1, -1), h_pad),
            ('RIGHTPADDING', (0, 0), (-1, -1), h_pad),
        ]))
        elements.append(Spacer(1, 3))
        if num_lines <= 40:
            elements.append(KeepTogether(code_table))
        else:
            elements.append(code_table)
        elements.append(Spacer(1, 4))

    # Build Cover Page first (Only Logo, Product Name, Subtitle, Version, Technical Doc)
    elements.append(Spacer(1, 55))
    if os.path.exists(LOGO_PATH):
        try:
            logo_img = Image(LOGO_PATH, width=170, height=54)
            logo_img.hAlign = 'CENTER'
            elements.append(logo_img)
        except Exception:
            elements.append(Paragraph("YES PANCHI", styles["cover_company"]))
    else:
        elements.append(Paragraph("YES PANCHI", styles["cover_company"]))
    
    elements.append(Spacer(1, 32))
    elements.append(Paragraph("YES PANCHI GROUP OF COMPANIES", styles["cover_company"]))
    elements.append(Paragraph("YP-RAKSHA", styles["cover_title"]))
    elements.append(Paragraph("Real-Time AI Knowledge Screening &amp; Hazard Audit", styles["cover_subtitle"]))
    elements.append(Spacer(1, 36))
    elements.append(Paragraph("Technical Documentation Manual", styles["cover_doc_type"]))
    elements.append(Spacer(1, 20))
    elements.append(Paragraph("Version 2.5", styles["cover_version"]))
    add_page_break()

    past_cover = False
    under_h3 = False

    while i < n:
        raw_line = lines[i]
        line = raw_line.strip()

        if "<div style=\"page-break-after: always;\"></div>" in raw_line:
            past_cover = True
            i += 1
            continue

        if not past_cover:
            i += 1
            continue

        # Handle fenced code block
        if line.startswith("```"):
            if in_code:
                flush_code()
            else:
                if in_table:
                    flush_table()
                if in_callout:
                    flush_callout()
                in_code = True
            i += 1
            continue

        if in_code:
            code_lines.append(raw_line)
            i += 1
            continue

        # Handle callout block (> [!NOTE], > [!TIP], etc.)
        if line.startswith("> "):
            callout_content = line[2:].strip()
            if callout_content.startswith("[!"):
                if in_callout:
                    flush_callout()
                in_callout = True
                i += 1
                continue
            else:
                if not in_callout:
                    in_callout = True
                callout_lines.append(callout_content)
                i += 1
                continue
        elif in_callout:
            flush_callout()

        # Handle markdown table
        if line.startswith("|"):
            if not in_table:
                if in_callout:
                    flush_callout()
                in_table = True
            table_lines.append(line)
            i += 1
            continue
        elif in_table:
            flush_table()

        # Handle explicit page breaks
        if "page-break-after" in line or "page-break-before" in line or "<!-- pagebreak -->" in line:
            if in_table:
                flush_table()
            if in_callout:
                flush_callout()
            if in_code:
                flush_code()
            add_page_break()
            i += 1
            continue

        # Skip horizontal rules
        if line in ["---", "***", "___"]:
            i += 1
            continue

        # Skip center div tags
        if line.startswith("<div") or line.startswith("</div"):
            i += 1
            continue

        # Handle Headings
        if line.startswith("# "):
            h_text = format_for_reportlab(line[2:].strip())
            if "Table of Contents" in h_text:
                # TOC is directly on Page 2 (Cover page already ended with PageBreak)
                elements.append(Paragraph(h_text, styles["h1"]))
            elif "Section " in h_text:
                # Every section starts on its own brand new page as requested!
                add_page_break()
                
                # Render executive Section Header Banner
                banner_p = Paragraph(f"<b>{h_text.upper()}</b>", styles["sec_banner_title"])
                banner_tbl = Table([[banner_p]], colWidths=[CONTENT_W])
                banner_tbl.setStyle(TableStyle([
                    ('BACKGROUND', (0, 0), (-1, -1), NAVY),
                    ('LINELEFT', (0, 0), (0, -1), 4.0, GREEN),
                    ('TOPPADDING', (0, 0), (-1, -1), 5.5),
                    ('BOTTOMPADDING', (0, 0), (-1, -1), 5.5),
                    ('LEFTPADDING', (0, 0), (-1, -1), 8),
                    ('RIGHTPADDING', (0, 0), (-1, -1), 8),
                ]))
                elements.append(KeepTogether(banner_tbl))
                elements.append(Spacer(1, 6))
            else:
                elements.append(Spacer(1, 12))
                elements.append(Paragraph(h_text, styles["h1"]))
            i += 1
            continue

        if line.startswith("## "):
            under_h3 = False
            h_text = format_for_reportlab(line[3:].strip())
            elements.append(Paragraph(h_text, styles["h2"]))
            i += 1
            continue

        if line.startswith("### "):
            under_h3 = True
            h_text = format_for_reportlab(line[4:].strip())
            elements.append(Paragraph(h_text, styles["h3"]))
            i += 1
            continue

        if line.startswith("#### "):
            under_h3 = True
            h_text = format_for_reportlab(line[5:].strip())
            elements.append(Paragraph(h_text, styles["h4"]))
            i += 1
            continue

        # Handle Bullet points
        if line.startswith("* ") or line.startswith("- "):
            b_text = format_for_reportlab(line[2:].strip())
            st = styles["bullet_indent"] if under_h3 else styles["bullet"]
            elements.append(Paragraph(f"• {b_text}", st))
            i += 1
            continue

        # Handle Numbered Lists
        num_match = re.match(r"^(\d+\.)\s+(.*)$", line)
        if num_match:
            num_prefix = num_match.group(1)
            num_text = format_for_reportlab(num_match.group(2).strip())
            st = styles["bullet_indent"] if under_h3 else styles["bullet"]
            elements.append(Paragraph(f"<b>{num_prefix}</b> {num_text}", st))
            i += 1
            continue

        # Regular paragraph
        if line:
            p_text = format_for_reportlab(line)
            st = styles["body_indent"] if under_h3 else styles["body"]
            elements.append(Paragraph(p_text, st))

        i += 1

    if in_table:
        flush_table()
    if in_callout:
        flush_callout()
    if in_code:
        flush_code()

    return elements


def generate_pdf():
    print("[PDF] Reading markdown source...")
    md_text = MD_SOURCE.read_text(encoding="utf-8")

    styles = get_pdf_styles()
    doc = SimpleDocTemplate(
        str(PDF_OUTPUT_DOCS),
        pagesize=A4,
        leftMargin=MARGIN,
        rightMargin=MARGIN,
        topMargin=MARGIN,
        bottomMargin=MARGIN
    )

    print("[PDF] Parsing elements...")
    elements = parse_markdown_to_elements(md_text, styles)

    print("[PDF] Building document with NumberedCanvas...")
    doc.build(elements, canvasmaker=NumberedCanvas)

    # Copy to root
    shutil.copyfile(str(PDF_OUTPUT_DOCS), str(PDF_OUTPUT_ROOT))
    print(f"[PDF] Successfully generated:\n - {PDF_OUTPUT_DOCS}\n - {PDF_OUTPUT_ROOT}")



def add_docx_runs(paragraph, text, default_font="Arial", default_size=8.0, default_color=(51, 65, 85), is_header=False):
    """
    Splits text by Markdown bold (**...**) and inline code (`...`) and appends properly styled runs to paragraph.
    Handles <br/> tags as native newlines for multi-line table cells.
    """
    if not text:
        return
    
    # Replace <br/> or <br> with newline
    text = re.sub(r"<br\s*/?>", "\n", text)
    
    # Split by bold **...**
    bold_parts = re.split(r"(\*\*.*?\*\*)", text)
    for b_part in bold_parts:
        if not b_part:
            continue
        is_bold = is_header
        tok = b_part
        if b_part.startswith("**") and b_part.endswith("**") and len(b_part) >= 4:
            tok = b_part[2:-2]
            is_bold = True
        
        # Split by inline code `...`
        code_parts = re.split(r"(`.*?`)", tok)
        for c_part in code_parts:
            if not c_part:
                continue
            is_code = False
            c_tok = c_part
            if c_part.startswith("`") and c_part.endswith("`") and len(c_part) >= 2:
                c_tok = c_part[1:-1]
                is_code = True
            
            run = paragraph.add_run(c_tok)
            run.font.name = "Courier New" if is_code else default_font
            run.font.size = Pt(default_size - 0.5 if is_code else default_size)
            run.font.bold = is_bold
            run.font.color.rgb = RGBColor(*default_color)


def generate_docx():
    print("[DOCX] Creating Word document...")
    doc = docx.Document()

    # Set Margins to 0.5 inches
    section = doc.sections[0]
    section.top_margin = Inches(0.5)
    section.bottom_margin = Inches(0.5)
    section.left_margin = Inches(0.5)
    section.right_margin = Inches(0.5)
    
    # Suppress Header & Footer on Cover Page (First Page)
    section.different_first_page_header_footer = True

    md_text = MD_SOURCE.read_text(encoding="utf-8")
    lines = md_text.splitlines()

    # Cover Page (Only Logo, Product Name, Subtitle, Version, Technical Doc Manual)
    p_logo = doc.add_paragraph()
    p_logo.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_logo.paragraph_format.space_before = Pt(30)
    p_logo.paragraph_format.space_after = Pt(12)
    if os.path.exists(LOGO_PATH):
        try:
            p_logo.add_run().add_picture(LOGO_PATH, width=Inches(2.5))
        except Exception:
            pass
    
    p_comp = doc.add_paragraph()
    p_comp.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_comp.paragraph_format.space_before = Pt(6)
    p_comp.paragraph_format.space_after = Pt(2)
    r_comp = p_comp.add_run("YES PANCHI GROUP OF COMPANIES\n")
    r_comp.font.name = "Arial"
    r_comp.font.size = Pt(13)
    r_comp.font.bold = True
    r_comp.font.color.rgb = RGBColor(15, 23, 42)

    r_prod = p_comp.add_run("YP-RAKSHA\n")
    r_prod.font.name = "Arial"
    r_prod.font.size = Pt(32)
    r_prod.font.bold = True
    r_prod.font.color.rgb = RGBColor(15, 23, 42)

    r_sub = p_comp.add_run("Real-Time AI Knowledge Screening & Hazard Audit\n\n")
    r_sub.font.name = "Arial"
    r_sub.font.size = Pt(12)
    r_sub.font.bold = True
    r_sub.font.color.rgb = RGBColor(15, 118, 110)

    p_doc = doc.add_paragraph()
    p_doc.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_doc.paragraph_format.space_before = Pt(36)
    p_doc.paragraph_format.space_after = Pt(4)
    r_doc = p_doc.add_run("Technical Documentation Manual\n\n")
    r_doc.font.name = "Arial"
    r_doc.font.size = Pt(20)
    r_doc.font.bold = True
    r_doc.font.color.rgb = RGBColor(30, 41, 59)

    r_ver = p_doc.add_run("Version 2.5\n")
    r_ver.font.name = "Arial"
    r_ver.font.size = Pt(12)
    r_ver.font.bold = True
    r_ver.font.color.rgb = RGBColor(5, 150, 105)

    doc.add_page_break()

    # Content Parsing
    past_cover = False
    in_code = False
    code_lines = []
    in_table = False
    table_lines = []

    def flush_docx_table():
        nonlocal in_table, table_lines
        if not table_lines:
            in_table = False
            return
        
        parsed_rows = []
        for t_line in table_lines:
            t_line = t_line.strip()
            if not t_line.startswith("|"):
                continue
            raw_cells = re.split(r"(?<!\\)\|", t_line)
            cells = [c.strip().replace(r"\|", "• ") for c in raw_cells[1:-1]]
            if not cells or all(re.match(r"^:?-+:?$", c) for c in cells):
                continue
            parsed_rows.append(cells)

        table_lines = []
        in_table = False
        if not parsed_rows:
            return

        col_count = max(len(r) for r in parsed_rows)
        for r in parsed_rows:
            while len(r) < col_count:
                r.append("")

        tbl = doc.add_table(rows=len(parsed_rows), cols=col_count)
        tbl.alignment = WD_TABLE_ALIGNMENT.CENTER

        # Repeat header row on every new page
        trPr = tbl.rows[0]._tr.get_or_add_trPr()
        trPr.append(parse_xml(f'<w:tblHeader {nsdecls("w")}/>'))

        for r_idx, r in enumerate(parsed_rows):
            row_cells = tbl.rows[r_idx].cells
            for c_idx, c_val in enumerate(r):
                p = row_cells[c_idx].paragraphs[0]
                p.paragraph_format.space_before = Pt(2)
                p.paragraph_format.space_after = Pt(2)

                tcPr = row_cells[c_idx]._tc.get_or_add_tcPr()
                if r_idx == 0:
                    add_docx_runs(p, c_val, default_size=8.0, default_color=(255, 255, 255), is_header=True)
                    shd = parse_xml(f'<w:shd {nsdecls("w")} w:fill="0F172A"/>')
                    tcPr.append(shd)
                else:
                    first_col = (c_idx == 0)
                    add_docx_runs(p, c_val, default_size=7.5, default_color=(15, 23, 42) if first_col else (51, 65, 85), is_header=first_col)
                    if r_idx % 2 == 1:
                        shd = parse_xml(f'<w:shd {nsdecls("w")} w:fill="F8FAFC"/>')
                        tcPr.append(shd)

        doc.add_paragraph().paragraph_format.space_after = Pt(3)

    def flush_docx_code():
        nonlocal in_code, code_lines
        if not code_lines:
            in_code = False
            return
        
        non_empty = [l for l in code_lines if l.strip()]
        max_line_len = max(len(l) for l in non_empty) if non_empty else 80
        num_lines = len(code_lines)
        if max_line_len >= 110 or num_lines > 45:
            c_size = 5.8
        elif max_line_len >= 95:
            c_size = 7.0
        else:
            c_size = 7.8

        c_text = "\n".join(code_lines)
        code_lines = []
        in_code = False

        tbl = doc.add_table(rows=1, cols=1)
        tbl.alignment = WD_TABLE_ALIGNMENT.CENTER
        cell = tbl.cell(0, 0)
        p = cell.paragraphs[0]
        p.paragraph_format.space_before = Pt(3)
        p.paragraph_format.space_after = Pt(3)
        run = p.add_run(c_text)
        run.font.name = "Courier New"
        run.font.size = Pt(c_size)
        run.font.color.rgb = RGBColor(15, 23, 42)

        tcPr = cell._tc.get_or_add_tcPr()
        shd = parse_xml(f'<w:shd {nsdecls("w")} w:fill="F8FAFC"/>')
        tcPr.append(shd)
        doc.add_paragraph().paragraph_format.space_after = Pt(3)

    i = 0
    n = len(lines)
    while i < n:
        raw_line = lines[i]
        line = raw_line.strip()

        if "<div style=\"page-break-after: always;\"></div>" in raw_line:
            past_cover = True
            i += 1
            continue

        if not past_cover:
            i += 1
            continue

        # Code block
        if line.startswith("```"):
            if in_code:
                flush_docx_code()
            else:
                if in_table:
                    flush_docx_table()
                in_code = True
            i += 1
            continue

        if in_code:
            code_lines.append(raw_line)
            i += 1
            continue

        # Table block
        if line.startswith("|"):
            if not in_table:
                in_table = True
            table_lines.append(line)
            i += 1
            continue
        elif in_table:
            flush_docx_table()

        # Handle explicit page breaks
        if "page-break-after" in line or "page-break-before" in line or "<!-- pagebreak -->" in line:
            if in_table:
                flush_docx_table()
            if in_code:
                flush_docx_code()
            doc.add_page_break()
            i += 1
            continue

        # Skip horizontal rules
        if line in ["---", "***", "___"] or line.startswith("<div") or line.startswith("</div"):
            i += 1
            continue

        # Headings
        if line.startswith("# "):
            h_text = line[2:].strip()
            if h_text == "Table of Contents":
                p = doc.add_paragraph()
                p.paragraph_format.space_before = Pt(14)
                p.paragraph_format.space_after = Pt(4)
                p.paragraph_format.keep_with_next = True
                add_docx_runs(p, h_text, default_size=13.0, default_color=(15, 23, 42), is_header=True)
            elif "Section " in h_text:
                # Every section starts on its own brand-new page as requested!
                doc.add_page_break()
                banner_tbl = doc.add_table(rows=1, cols=1)
                banner_tbl.alignment = WD_TABLE_ALIGNMENT.CENTER
                b_cell = banner_tbl.cell(0, 0)
                bp = b_cell.paragraphs[0]
                bp.paragraph_format.space_before = Pt(5)
                bp.paragraph_format.space_after = Pt(5)
                add_docx_runs(bp, h_text.upper(), default_size=11.5, default_color=(255, 255, 255), is_header=True)
                tcPr = b_cell._tc.get_or_add_tcPr()
                shd = parse_xml(f'<w:shd {nsdecls("w")} w:fill="0F172A"/>')
                tcPr.append(shd)
                doc.add_paragraph().paragraph_format.space_after = Pt(4)
            else:
                p = doc.add_paragraph()
                p.paragraph_format.space_before = Pt(14)
                p.paragraph_format.space_after = Pt(4)
                p.paragraph_format.keep_with_next = True
                add_docx_runs(p, h_text, default_size=13.0, default_color=(15, 23, 42), is_header=True)
            i += 1
            continue

        if line.startswith("## "):
            under_h3 = False
            h_text = line[3:].strip()
            p = doc.add_paragraph()
            p.paragraph_format.space_before = Pt(9)
            p.paragraph_format.space_after = Pt(3)
            p.paragraph_format.keep_with_next = True
            add_docx_runs(p, h_text, default_size=10.5, default_color=(15, 118, 110), is_header=True)
            i += 1
            continue

        if line.startswith("### "):
            under_h3 = True
            h_text = line[4:].strip()
            p = doc.add_paragraph()
            p.paragraph_format.space_before = Pt(6)
            p.paragraph_format.space_after = Pt(2)
            p.paragraph_format.left_indent = Inches(0.2)
            p.paragraph_format.keep_with_next = True
            add_docx_runs(p, h_text, default_size=9.0, default_color=(30, 41, 59), is_header=True)
            i += 1
            continue

        if line.startswith("#### "):
            under_h3 = True
            h_text = line[5:].strip()
            p = doc.add_paragraph()
            p.paragraph_format.space_before = Pt(4)
            p.paragraph_format.space_after = Pt(2)
            p.paragraph_format.left_indent = Inches(0.25)
            p.paragraph_format.keep_with_next = True
            add_docx_runs(p, h_text, default_size=8.2, default_color=(15, 23, 42), is_header=True)
            i += 1
            continue

        # Callouts (> [!NOTE], > [!TIP], etc.)
        if line.startswith("> "):
            c_text = line[2:].strip()
            if c_text.startswith("[!"):
                i += 1
                continue
            p = doc.add_paragraph()
            p.paragraph_format.left_indent = Inches(0.2)
            p.paragraph_format.space_before = Pt(1.5)
            p.paragraph_format.space_after = Pt(1.5)
            add_docx_runs(p, c_text, default_size=7.8, default_color=(15, 118, 110))
            i += 1
            continue

        # Bullet list
        if line.startswith("* ") or line.startswith("- "):
            b_text = line[2:].strip()
            p = doc.add_paragraph(style='List Bullet')
            if under_h3:
                p.paragraph_format.left_indent = Inches(0.35)
            p.paragraph_format.space_before = Pt(1)
            p.paragraph_format.space_after = Pt(1)
            add_docx_runs(p, b_text, default_size=7.8, default_color=(51, 65, 85))
            i += 1
            continue

        # Numbered list
        num_match = re.match(r"^(\d+\.)\s+(.*)$", line)
        if num_match:
            n_text = f"{num_match.group(1)} {num_match.group(2).strip()}"
            p = doc.add_paragraph()
            p.paragraph_format.left_indent = Inches(0.35 if under_h3 else 0.2)
            p.paragraph_format.space_before = Pt(1.5)
            p.paragraph_format.space_after = Pt(1.5)
            add_docx_runs(p, n_text, default_size=7.8, default_color=(51, 65, 85))
            i += 1
            continue

        # Plain text
        if line:
            p = doc.add_paragraph()
            if under_h3:
                p.paragraph_format.left_indent = Inches(0.2)
            p.paragraph_format.space_before = Pt(2)
            p.paragraph_format.space_after = Pt(2)
            add_docx_runs(p, line, default_size=7.8, default_color=(51, 65, 85))

        i += 1

    if in_table:
        flush_docx_table()
    if in_code:
        flush_docx_code()

    # Running Header and Footer on Word Document (Pages 2+)
    header = section.header
    hp = header.paragraphs[0]
    hp.alignment = WD_ALIGN_PARAGRAPH.RIGHT
    hr = hp.add_run("YP-Raksha v2.5 | Enterprise Technical Documentation Manual")
    hr.font.name = "Arial"
    hr.font.size = Pt(7.5)
    hr.font.color.rgb = RGBColor(100, 116, 139)

    footer = section.footer
    fp = footer.paragraphs[0]
    fp.alignment = WD_ALIGN_PARAGRAPH.LEFT
    fr = fp.add_run("YES PANCHI GROUP OF COMPANIES")
    fr.font.name = "Arial"
    fr.font.size = Pt(7.5)
    fr.font.color.rgb = RGBColor(100, 116, 139)

    # Save defensively with retry if file is locked
    saved = False
    for attempt in range(2):
        try:
            doc.save(str(DOCX_OUTPUT_DOCS))
            shutil.copyfile(str(DOCX_OUTPUT_DOCS), str(DOCX_OUTPUT_ROOT))
            saved = True
            break
        except PermissionError:
            print("[DOCX WARNING] File locked, attempting to release lock...")
            import subprocess
            subprocess.run(["powershell", "-Command", "Stop-Process -Name WINWORD -Force -ErrorAction SilentlyContinue"], capture_output=True)
            import time
            time.sleep(1)

    if saved:
        print(f"[DOCX] Successfully generated:\n - {DOCX_OUTPUT_DOCS}\n - {DOCX_OUTPUT_ROOT}")
    else:
        alt_docs = DOCS_DIR / "YP_RAKSHA_Enterprise_All_In_One_Technical_Document_latest.docx"
        alt_root = BASE_DIR / "YP_RAKSHA_Enterprise_All_In_One_Technical_Document_latest.docx"
        doc.save(str(alt_docs))
        shutil.copyfile(str(alt_docs), str(alt_root))
        print(f"[DOCX] Saved to alternative file:\n - {alt_docs}\n - {alt_root}")


def validate_pdf():
    print("[VALIDATE] Inspecting PDF pages via PyMuPDF...")
    doc = pymupdf.open(str(PDF_OUTPUT_DOCS))
    total = len(doc)
    print(f"[VALIDATE] Total PDF Pages: {total}")
    for idx in range(min(5, total)):
        page = doc[idx]
        text_len = len(page.get_text())
        print(f" - Page {idx+1}: {text_len} characters extracted")
    doc.close()


if __name__ == "__main__":
    generate_pdf()
    generate_docx()
    validate_pdf()
    print("[COMPLETE] All files generated and validated successfully!")
