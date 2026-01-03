setup:
	@if [ ! -d env ]; then \
		python -m venv env; \
	fi
	@if [ ! -x env/bin/mkdocs ]; then \
		env/bin/pip install -r requirements.txt --no-cache-dir; \
	fi	
docgen: setup
	@env/bin/mkdocs build 
docrun: setup
	@env/bin/mkdocs serve
