import pypdf

# Read V1.04
print('=== V1.04 PDF Content ===')
try:
    pdf_v104 = pypdf.PdfReader(r'c:\GitHub\mbfvsolutionsllc\electriccompare\facts\Electricity Facts Label_V1.04.pdf')
    print(f'Pages: {len(pdf_v104.pages)}')
    for i, page in enumerate(pdf_v104.pages):
        print(f'\n--- Page {i+1} ---')
        text = page.extract_text()
        print(text if text else 'No text extracted')
except Exception as e:
    print(f'Error reading V1.04: {e}')

print('\n\n=== V1.14 PDF Content ===')
# Read V1.14
try:
    pdf_v114 = pypdf.PdfReader(r'c:\GitHub\mbfvsolutionsllc\electriccompare\facts\Electricity Facts Label_V1.14.pdf')
    print(f'Pages: {len(pdf_v114.pages)}')
    for i, page in enumerate(pdf_v114.pages):
        print(f'\n--- Page {i+1} ---')
        text = page.extract_text()
        print(text if text else 'No text extracted')
except Exception as e:
    print(f'Error reading V1.14: {e}')
