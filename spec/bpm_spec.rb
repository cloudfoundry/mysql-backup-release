require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'streaming-mysql-backup-tool job' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '..')) }
  let(:job) { release.job('streaming-mysql-backup-tool') }

  describe 'bpm template' do
    let(:template) { job.template('config/bpm.yml') }

    context('when xtrabackup_path is provided') do
      let(:spec) {{
        "cf-mysql-backup" => {
          'xtrabackup_path' => 'VALUE'
        }
      }}

      it 'renders the xtrabackup path' do
        bpm_output = template.render(spec)
        bpm_yaml = YAML.load(bpm_output)
        expect(bpm_yaml['processes'].first['env']['PATH']).to eq('/bin:/usr/bin:/sbin:/usr/sbin:VALUE/xtrabackup/bin')
      end
    end
  end
end
