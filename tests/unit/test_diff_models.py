from lazylab.models.gitlab import MRDiffData, MRDiffFile


class TestMRDiffFile:
    def test_modified_file_defaults(self):
        f = MRDiffFile(old_path="a.py", new_path="a.py", diff="@@ -1 +1 @@\n-old\n+new")
        assert not f.new_file
        assert not f.deleted_file
        assert not f.renamed_file
        assert f.diff.startswith("@@")

    def test_new_file(self):
        f = MRDiffFile(old_path="/dev/null", new_path="b.py", diff="+line", new_file=True)
        assert f.new_file
        assert not f.deleted_file

    def test_deleted_file(self):
        f = MRDiffFile(old_path="c.py", new_path="c.py", diff="-line", deleted_file=True)
        assert f.deleted_file
        assert not f.new_file

    def test_renamed_file(self):
        f = MRDiffFile(old_path="old.py", new_path="new.py", diff="", renamed_file=True)
        assert f.renamed_file
        assert f.old_path != f.new_path

    def test_empty_diff(self):
        f = MRDiffFile(old_path="bin.dat", new_path="bin.dat", diff="")
        assert f.diff == ""


class TestMRDiffData:
    def test_empty_files(self):
        data = MRDiffData()
        assert data.files == []

    def test_with_files(self):
        files = [
            MRDiffFile(old_path="a.py", new_path="a.py", diff="+x"),
            MRDiffFile(old_path="b.py", new_path="b.py", diff="-y"),
        ]
        data = MRDiffData(files=files)
        assert len(data.files) == 2
        assert data.files[0].new_path == "a.py"
        assert data.files[1].new_path == "b.py"
