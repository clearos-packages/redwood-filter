# Redwood Content Filter RPM spec
%global debug_package %{nil}

Name: redwood-filter
Version: 1.0
Release: 1%{dist}
Vendor: ClearFoundation
License: GPL
Group: System Environment/Daemons
Packager: ClearFoundation
Source: %{name}-%{version}.tar.gz
BuildRoot: /var/tmp/%{name}-%{version}
Summary: Redwood Content Filter
BuildRequires: golang git mercurial
Requires: initscripts /sbin/service
Requires(pre): /sbin/ldconfig, /usr/sbin/useradd, /usr/bin/getent
Requires(postun): /usr/sbin/userdel
Requires(post): /sbin/chkconfig
Requires(preun): /sbin/chkconfig
Patch0: redwood-filter-clearos.patch

%description
Redwood Content Filter
Copyright (c) 2011-2012 Andy Balholm. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

Report bugs to: http://www.clearfoundation.com/docs/developer/bug_tracker/

%prep

%setup -q
mkdir -vp $(pwd)/go/{bin,pkg,src}
#GOPATH=$(pwd)/go go get code.google.com/p/redwood-filter
%patch0

%build
#GOPATH=$(pwd)/go go install code.google.com/p/redwood-filter
GOPATH=$(pwd)/go go install code.google.com/p/redwood-filter

# Install
%install
mkdir -vp %{buildroot}/%{_sbindir}
mkdir -vp %{buildroot}/%{_sysconfdir}
mkdir -vp %{buildroot}/%{_localstatedir}/log/redwood-filter
install -D -m 755 go/bin/redwood-filter %{buildroot}/%{_sbindir}/redwood-filter
cp -vr go/src/code.google.com/p/redwood-filter/config %{buildroot}/%{_sysconfdir}/redwood-filter

%if "0%{dist}" == "0.v6"
install -D -m 644 go/src/code.google.com/p/redwood-filter/startup/redhat %{buildroot}/%{_sysconfdir}/init.d/redwood-filter
%else
install -D -m 644 redwood-filter.service %{buildroot}/lib/systemd/system/redwood-filter.service
install -D -m 644 redwood-filter.tmp %{buildroot}/%{_tmpfilesdir}/redwood-filter.conf
%endif

# Clean-up
%clean
[ "%{buildroot}" != / ] && rm -rf "%{buildroot}"

# Pre install
%pre
/usr/bin/getent passwd redwood 2>&1 >/dev/null ||\
    /usr/sbin/useradd -M -c "Redwood Filter" -r -d %{_sbindir}/redwood-filter -s /bin/false redwood 2> /dev/null || :

%preun
%if "0%{dist}" == "0.v6"
if [ "$1" = 0 ]; then
    /sbin/chkconfig --del redwood-filter
fi
%else
if [ "$1" = 0 ]; then
    /sbin/chkconfig --del redwood-filter
    /usr/bin/systemctl stop redwood-filter.service -q
    /usr/bin/systemctl disable redwood-filter.service -q
fi
%endif

# Post install
%post
%if "0%{dist}" == "0.v6"
/sbin/chkconfig --add redwood-filter >/dev/null 2>&1 || :
/sbin/service redwood-filter condrestart >/dev/null 2>&1 || :
%else
/sbin/chkconfig --add redwood-filter >/dev/null 2>&1 || :
/usr/bin/systemctl enable redwood-filter.service -q
/usr/bin/systemctl reload-or-restart redwood-filter.service -q
%endif

# Post uninstall
%postun
%if "0%{dist}" == "0.v6"
if [ -f /var/lock/subsys/redwood-filter ]; then
    killall -TERM redwood-filter 2>&1 >/dev/null || :
    sleep 2
fi
%endif

# Files
%files
%defattr(-,root,root)
%if "0%{dist}" == "0.v6"
%attr(755,root,root) %{_sysconfdir}/init.d/redwood-filter
%else
%attr(755,root,root) /lib/systemd/system
%attr(755,root,root) %{_tmpfilesdir}
%endif
%{_sbindir}/redwood-filter
%{_sysconfdir}/redwood-filter
%attr(755,redwood,redwood) %{_localstatedir}/log/redwood-filter

# vi: expandtab shiftwidth=4 softtabstop=4 tabstop=4
