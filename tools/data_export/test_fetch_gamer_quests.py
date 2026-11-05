import unittest
from fetch_gamer_quests import parse, linked_title, factual_fields


class GamerQuestResearchTest(unittest.TestCase):
    def test_article_scope_excludes_sidebar_and_scripts(self):
        raw = '''<a href="/wiki.php?n=9045:Sidebar">Sidebar</a>
        <div class="wikiContent"><script>malicious()</script><a name="任務"></a>
        <h2>任務</h2><div>觸發條件：等級30以上</div><br>任務獎勵：星星*2
        <a href="/wiki.php?n=9045:Context">Context</a></div>
        <a href="/wiki.php?n=9045:Footer">Footer</a>'''.encode()
        parsed = parse(raw)
        self.assertEqual([label for _, label in parsed.links], ['', 'Context'])
        sections = parsed.finish()
        self.assertNotIn('malicious()', ''.join(''.join(s['text']) for s in sections))
        self.assertEqual((sections[-1]['title'], sections[-1]['anchor']), ('任務', '任務'))
        facts = factual_fields(sections[-1])
        self.assertEqual(facts['prerequisites'], '等級30以上')

    def test_links_preserve_shared_namespace_and_reject_external_games(self):
        self.assertEqual(linked_title('//wiki2.gamer.com.tw/wiki.php?n=9045%3A一般任務&ss=113640&f=M'), '一般任務')
        self.assertIsNone(linked_title('https://example.com/wiki.php?n=9045:任務'))
        self.assertIsNone(linked_title('/wiki.php?n=9999:任務'))
        self.assertIsNone(linked_title('/recentEdit.php?n=9045:任務'))

    def test_facts_exclude_walkthrough_prose(self):
        fields = factual_fields(dict(text=['觸發地點：北島\n觸發人物: NPC\n任務流程：Do something\n',
                                         '任務獎勵：道具*2\n觸發條件：無\n']))
        self.assertEqual(fields, dict(location='北島', npc='NPC', prerequisites='無', required_items='', rewards='道具*2'))

    def test_menu_mode_discovers_links_outside_article_body(self):
        parsed = parse(b'<div><a href="/wiki.php?n=9045:Quest">Quest</a></div>', menu=True)
        self.assertEqual(parsed.links, [['/wiki.php?n=9045:Quest', 'Quest']])


if __name__ == '__main__':
    unittest.main()
